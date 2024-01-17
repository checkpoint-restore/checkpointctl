#define _GNU_SOURCE
#include <stdio.h>
#include <unistd.h>
#include <sys/mman.h>
#include <stdlib.h>
#include <signal.h>
#include <sched.h>
#include <fcntl.h>
#include <stdbool.h>
#include <string.h>
#include <arpa/inet.h>
#include <sys/prctl.h>

#define STKS	(4*4096)

#ifndef CLONE_NEWPID
#define CLONE_NEWPID    0x20000000
#endif

#define SERVER_IP "127.0.0.1"
#define PORT 5000
#define MAX_BUFFER_SIZE 1024

typedef struct {
	char *log_file;
	bool use_tcp_socket;
} opts_t;

void run_tcp_server(void)
{
	int server_socket, client_socket, ret;
	struct sockaddr_in server_address, client_address;
	char buffer[MAX_BUFFER_SIZE];
	const int enable = 1;

	ret = fork();
	if (ret < 0) {
		perror("tcp-server: fork failed");
		return;
	}

	if (ret > 0) {
		return;
	}

	prctl(PR_SET_NAME, "tcp-server");

	server_socket = socket(AF_INET, SOCK_STREAM, 0);
	if (server_socket == -1) {
		perror("tcp-server: Socket creation failed");
		exit(EXIT_FAILURE);
	}

	setsockopt(server_socket, SOL_SOCKET, SO_REUSEADDR, &(int){1}, sizeof(int));
	setsockopt(server_socket, SOL_SOCKET, SO_REUSEPORT, &(int){1}, sizeof(int));

	server_address.sin_family = AF_INET;
	server_address.sin_addr.s_addr = INADDR_ANY;
	server_address.sin_port = htons(PORT);

	if (bind(server_socket, (struct sockaddr*)&server_address, sizeof(server_address)) == -1) {
		perror("tcp-server: Socket bind failed");
		exit(EXIT_FAILURE);
	}

	if (listen(server_socket, 5) == -1) {
		perror("tcp-server: Listen failed");
		exit(EXIT_FAILURE);
	}

	socklen_t client_address_len = sizeof(client_address);
	client_socket = accept(server_socket, (struct sockaddr*)&client_address, &client_address_len);
	if (client_socket == -1) {
		perror("tcp-server: Accept failed");
		exit(EXIT_FAILURE);
	}

	while (1) {
		memset(buffer, 0, sizeof(buffer));
		recv(client_socket, buffer, sizeof(buffer), 0);
	}

	close(server_socket);
	close(client_socket);
	exit(0);
}

void run_tcp_client(void)
{
	int client_socket, max_connection_tries = 5;
	struct sockaddr_in server_address;
	char buffer[MAX_BUFFER_SIZE];
	bool connected = false, ret;

	ret = fork();
	if (ret < 0) {
		perror("tcp-client: fork failed");
		return;
	}

	if (ret > 0) {
		return;
	}

	prctl(PR_SET_NAME, "tcp-client");

	client_socket = socket(AF_INET, SOCK_STREAM, 0);
	if (client_socket == -1) {
		perror("Socket creation failed");
		exit(EXIT_FAILURE);
	}

	server_address.sin_family = AF_INET;
	server_address.sin_port = htons(PORT);

	if (inet_pton(AF_INET, SERVER_IP, &server_address.sin_addr) <= 0) {
		perror("tcp-client: Invalid address");
		exit(EXIT_FAILURE);
	}

	while (!connected) {
		if (connect(client_socket, (struct sockaddr*)&server_address, sizeof(server_address)) == -1) {
			if (max_connection_tries > 0) {
				max_connection_tries--;
				continue;
			}

			perror("tcp-client: Connection failed");
			exit(EXIT_FAILURE);
		}
		connected = true;
	}

	while (1) {
		send(client_socket, "ping", 5, 0);
		/* Send messages every second */
		sleep(1);
	}

	close(client_socket);
	exit(0);
}

static int do_test(void *opts_ptr)
{
	opts_t *opts = opts_ptr;
	int fd, ret, i = 0;

	setsid();

	close(0);
	close(1);
	close(2);

	fd = open("/dev/null", O_RDONLY);
	if (fd != 0) {
		dup2(fd, 0);
		close(fd);
	}

	if (opts->log_file) {
		fd = open(opts->log_file, O_WRONLY | O_TRUNC | O_CREAT, 0600);
		dup2(fd, 1);
		dup2(fd, 2);
		if (fd != 1 && fd != 2)
			close(fd);
	}

	if (opts->use_tcp_socket) {
		if (unshare(CLONE_NEWNET))
			return 1;
		if (system("ip link set up dev lo"))
			return 1;
		run_tcp_server();
		run_tcp_client();
	}

	while (1) {
		sleep(1);
		printf("%d\n", i++);
		fflush(stdout);
	}

	return 0;
}

static int parse_options(int argc, char **argv, bool *usage_error, opts_t *opts)
{
	int i = 1, exit_code = -1;

	while (i < argc) {
		if ((!strcmp(argv[i], "--help")) || (!strcmp(argv[i], "-h"))) {
			*usage_error = false;
			return 1;
		}

		if ((!strcmp(argv[i], "--log-file")) || (!strcmp(argv[i], "-o"))) {
			opts->log_file = argv[i + 1];
			i += 2;
			continue;
		}

		if ((!strcmp(argv[i], "--tcp-socket")) || (!strcmp(argv[i], "-t"))) {
			opts->use_tcp_socket = true;
			i++;
			continue;
		}

		printf("Unknown option: %s\n", argv[i]);
		*usage_error = true;
		goto out;
	}

	exit_code = 0;
out:
	return exit_code;
}

int main(int argc, char **argv) {
	void *stk;
	int i, pid, log_fd, option;
	bool usage_error = false;
	opts_t opts = {NULL};
	int ret;

	ret = parse_options(argc, argv, &usage_error, &opts);
	if (ret) {
		fprintf(stderr, "Usage: %s -o/--log-file <log_file> [-t/--tcp-socket]\n", argv[0]);
		return (usage_error != false);
	}

	stk = mmap(NULL, STKS, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON | MAP_GROWSDOWN, 0, 0);
	pid = clone(do_test, stk + STKS, SIGCHLD | CLONE_NEWPID, (void*)&opts);
	if (pid < 0) {
		fprintf(stderr, "clone() failed: %m\n");
		return 1;
	}

	/* Wait for TCP sockets to be created if requested */
	if (opts.use_tcp_socket) {
		sleep(3);
	}
	printf("%d\n", pid);

	if (opts.log_file) {
		log_fd = open(opts.log_file, O_WRONLY | O_CREAT | O_TRUNC, 0666);
		if (log_fd == -1) {
			perror("Error opening log file");
			return 1;
		}

		dup2(log_fd, STDOUT_FILENO);
		dup2(log_fd, STDERR_FILENO);

		close(log_fd);
	}

	return 0;
}
