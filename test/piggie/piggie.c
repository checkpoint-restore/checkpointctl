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
#include <sys/socket.h>
#include <sys/types.h>
#include <errno.h>
#include <sys/wait.h>

#define STKS	(4*4096)
#define MAX_EXTRA_CLIENTS 16

#ifndef CLONE_NEWPID
#define CLONE_NEWPID    0x20000000
#endif

#define SERVER_IP "127.0.0.1"
#define PORT 5000
#define UDP_PORT 5001
#define MAX_BUFFER_SIZE 1024

typedef struct {
	char *log_file;
	bool use_tcp_socket;
	bool create_zombie;
} opts_t;

static pid_t tcp_extras[MAX_EXTRA_CLIENTS];
static int tcp_extras_n = 0;
static pid_t udp_extras[MAX_EXTRA_CLIENTS];
static int udp_extras_n = 0;

static int spawn_tcp_client(char *err, size_t errsz);
static int kill_tcp_client(char *err, size_t errsz);
static int spawn_udp_client(char *err, size_t errsz);
static int kill_udp_client(char *err, size_t errsz);

/*
 * Command FIFO actions allowing to change the piggie process state at
 * runtime. This mechanism allows to test the `checkpointctl diff`
 * functionality by modifying the process state (e.g., adding/removing
 * socket connections) between checkpoints.
 *
 * The action functions are triggered by writing a command name into the
 * command FIFO ($PIGGIE_CMD_FIFO). The corresponding function is then
 * executed, and its result is sent back through the acknowledgment
 * FIFO ($PIGGIE_ACK_FIFO) as either "ok" or "fail: <reason>".
 */
static struct {
	const char *name;
	int (*fn)(char *err, size_t errsz);
} actions[] = {
	{ "spawn-tcp-client", spawn_tcp_client },
	{ "kill-tcp-client",  kill_tcp_client  },
	{ "spawn-udp-client", spawn_udp_client },
	{ "kill-udp-client",  kill_udp_client  },
};

#define N_ACTIONS ((int)(sizeof(actions) / sizeof(actions[0])))

/*
 * Fork a child that creates an AF_INET socket of `sock_type`, connects
 * it to 127.0.0.1:port, and parks in pause(). The child signals back on
 * a sync pipe once connect() has returned so the caller knows the
 * kernel-level state is in place before acking.
 */
static int spawn_extra(int sock_type, int port, const char *name,
		       pid_t *stack, int *stack_n,
		       char *err, size_t errsz)
{
	int sync_pipe[2];
	pid_t p;
	char ready;
	ssize_t n;

	if (*stack_n >= MAX_EXTRA_CLIENTS) {
		snprintf(err, errsz, "stack full (max=%d)", MAX_EXTRA_CLIENTS);
		return -1;
	}

	if (pipe(sync_pipe) == -1) {
		snprintf(err, errsz, "pipe: %s", strerror(errno));
		return -1;
	}

	p = fork();
	if (p < 0) {
		close(sync_pipe[0]);
		close(sync_pipe[1]);
		snprintf(err, errsz, "fork: %s", strerror(errno));
		return -1;
	}

	if (p == 0) {
		int fd;
		char ok = 1;
		struct sockaddr_in addr = {
			.sin_family = AF_INET,
			.sin_port = htons(port),
		};

		close(sync_pipe[0]);
		prctl(PR_SET_NAME, name);
		fd = socket(AF_INET, sock_type, 0);
		if (fd == -1) _exit(1);
		if (inet_pton(AF_INET, SERVER_IP, &addr.sin_addr) <= 0) _exit(1);
		if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) == -1) _exit(1);
		if (write(sync_pipe[1], &ok, 1) != 1) _exit(1);
		close(sync_pipe[1]);
		while (1)
			pause();
	}

	close(sync_pipe[1]);
	n = read(sync_pipe[0], &ready, 1);
	close(sync_pipe[0]);
	if (n != 1) {
		waitpid(p, NULL, 0);
		snprintf(err, errsz, "%s failed to connect", name);
		return -1;
	}
	stack[(*stack_n)++] = p;
	return 0;
}

static int kill_extra(pid_t *stack, int *stack_n, char *err, size_t errsz)
{
	pid_t p;

	if (*stack_n == 0) {
		snprintf(err, errsz, "no extra clients");
		return -1;
	}

	p = stack[--(*stack_n)];
	kill(p, SIGTERM);
	waitpid(p, NULL, 0);
	return 0;
}

static int spawn_tcp_client(char *err, size_t errsz)
{
	return spawn_extra(SOCK_STREAM, PORT, "tcp-client2",
			   tcp_extras, &tcp_extras_n, err, errsz);
}

static int kill_tcp_client(char *err, size_t errsz)
{
	return kill_extra(tcp_extras, &tcp_extras_n, err, errsz);
}

static int spawn_udp_client(char *err, size_t errsz)
{
	return spawn_extra(SOCK_DGRAM, UDP_PORT, "udp-client2",
			   udp_extras, &udp_extras_n, err, errsz);
}

static int kill_udp_client(char *err, size_t errsz)
{
	return kill_extra(udp_extras, &udp_extras_n, err, errsz);
}

static void create_zombie(void)
{
	pid_t zombie_pid = fork();

	if (zombie_pid < 0) {
		perror("zombie: fork failed");
		return;
	}

	if (zombie_pid == 0) {
		/* This process will become the zombie */

		prctl(PR_SET_NAME, "piggie-zombie");

		/* First alive child */
		pid_t c1 = fork();
		if (c1 == 0) {
			prctl(PR_SET_NAME, "stopped-child");
			raise(SIGSTOP);   /* enters stopped task state */
			while (1)
				sleep(1);
		}

		/* Second alive child */
		pid_t c2 = fork();
		if (c2 == 0) {
			prctl(PR_SET_NAME, "alive-child");
			while (1)
				sleep(1);
		}

		/*
		 * Parent of the two children exits immediately.
		 * Since *its* parent does not wait(), this
		 * process becomes a zombie.
		 */
		_exit(0);
	}

	/*
	 * Original parent intentionally does NOT wait()
	 * zombie_pid stays as a zombie.
	 */
}

void run_tcp_server(void)
{
	int server_socket, ret;
	struct sockaddr_in server_address = {0};
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
	signal(SIGCHLD, SIG_IGN);

	server_socket = socket(AF_INET, SOCK_STREAM, 0);
	if (server_socket == -1) {
		perror("tcp-server: Socket creation failed");
		exit(EXIT_FAILURE);
	}

	setsockopt(server_socket, SOL_SOCKET, SO_REUSEADDR, &enable, sizeof(enable));
	setsockopt(server_socket, SOL_SOCKET, SO_REUSEPORT, &enable, sizeof(enable));

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

	while (1) {
		int client_socket = accept(server_socket, NULL, NULL);
		if (client_socket == -1) {
			if (errno == EINTR)
				continue;
			perror("tcp-server: Accept failed");
			exit(EXIT_FAILURE);
		}

		pid_t handler = fork();
		if (handler == 0) {
			char buffer[MAX_BUFFER_SIZE];
			prctl(PR_SET_NAME, "tcp-conn");
			close(server_socket);
			while (recv(client_socket, buffer, sizeof(buffer), 0) > 0) {}
			close(client_socket);
			_exit(0);
		}
		close(client_socket);
	}
}

void run_tcp_client(void)
{
	int client_socket, max_connection_tries = 50;
	struct sockaddr_in server_address = {0};
	char buffer[MAX_BUFFER_SIZE];
	const char msg[] = "ping";
	bool connected = false;
	pid_t ret;

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
				usleep(50000);  /* 50ms between retries */
				continue;
			}

			perror("tcp-client: Connection failed");
			exit(EXIT_FAILURE);
		}
		connected = true;
	}

	while (1) {
		ssize_t sent = send(client_socket, msg, sizeof(msg) - 1, 0);
		if (sent == -1) {
			perror("send");
		}

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
		if (fd < 0) {
			perror("open");
			return -1;
		}

		dup2(fd, STDOUT_FILENO);
		dup2(fd, STDERR_FILENO);

		if (fd != STDOUT_FILENO && fd != STDERR_FILENO) {
			close(fd);
		}
	} else {
		/*
		 * Reserve fd 1 and 2 with /dev/null so later opens don't
		 * land on them (e.g. the command/ack FIFOs below) -- that
		 * would otherwise hijack printf(stdout) writes.
		 */
		fd = open("/dev/null", O_WRONLY);
		if (fd >= 0) {
			dup2(fd, 1);
			dup2(fd, 2);
			if (fd != 1 && fd != 2)
				close(fd);
		}
	}

	if (opts->use_tcp_socket) {
		if (unshare(CLONE_NEWNET))
			return 1;
		if (system("ip link set up dev lo"))
			return 1;
		run_tcp_server();
		run_tcp_client();
	}

	if (opts->create_zombie) {
		create_zombie();
	}

	/*
	 * Optional synchronous command channel. The test script creates two
	 * FIFOs, points $PIGGIE_CMD_FIFO / $PIGGIE_ACK_FIFO at them, then for
	 * each command writes one line into cmd and reads one line from ack
	 * ("ok" or "fail: <reason>"). Both are opened O_RDWR so the script's
	 * briefly-closed write/read ends don't produce EOFs here.
	 */
	int cmd_fd = -1, ack_fd = -1;
	const char *cmd_path = getenv("PIGGIE_CMD_FIFO");
	const char *ack_path = getenv("PIGGIE_ACK_FIFO");
	if (cmd_path) {
		cmd_fd = open(cmd_path, O_RDWR | O_NONBLOCK);
		if (cmd_fd < 0) perror("piggie: open PIGGIE_CMD_FIFO");
	}
	if (ack_path) {
		ack_fd = open(ack_path, O_RDWR);
		if (ack_fd < 0) perror("piggie: open PIGGIE_ACK_FIFO");
	}

	while (1) {
		struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
		fd_set rfds;
		int maxfd = -1;

		FD_ZERO(&rfds);
		if (cmd_fd >= 0) {
			FD_SET(cmd_fd, &rfds);
			maxfd = cmd_fd;
		}

		if (select(maxfd + 1, maxfd >= 0 ? &rfds : NULL, NULL, NULL, &tv) > 0
		    && cmd_fd >= 0 && FD_ISSET(cmd_fd, &rfds)) {
			char buf[256];
			ssize_t n = read(cmd_fd, buf, sizeof(buf) - 1);
			if (n > 0) {
				buf[n] = 0;
				char *line, *saveptr = NULL;
				for (line = strtok_r(buf, "\n", &saveptr); line;
				     line = strtok_r(NULL, "\n", &saveptr)) {
					char err[256] = {0}, resp[288];
					int rc = -1, rlen;
					for (int a = 0; a < N_ACTIONS; a++) {
						if (strcmp(line, actions[a].name) == 0) {
							rc = actions[a].fn(err, sizeof(err));
							break;
						}
					}
					if (rc == 0) rlen = snprintf(resp, sizeof(resp), "ok\n");
					else if (*err) rlen = snprintf(resp, sizeof(resp), "fail: %s\n", err);
					else rlen = snprintf(resp, sizeof(resp), "fail: unknown command '%s'\n", line);
					if (ack_fd >= 0 && rlen > 0)
						(void)!write(ack_fd, resp, rlen);
				}
			}
		}

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

		if (!strcmp(argv[i], "--zombie") || !strcmp(argv[i], "-z")) {
			opts->create_zombie = true;
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
	opts_t opts = {0};
	int ret;

	ret = parse_options(argc, argv, &usage_error, &opts);
	if (ret) {
		fprintf(stderr, "Usage: %s -o/--log-file <log_file> [-t/--tcp-socket] [-z|--zombie]\n", argv[0]);
		return (usage_error != false);
	}

	stk = mmap(NULL, STKS, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON | MAP_GROWSDOWN, 0, 0);
	pid = clone(do_test, (char *)stk + STKS, SIGCHLD | CLONE_NEWPID, (void*)&opts);
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
