#define _GNU_SOURCE
#include <stdio.h>
#include <unistd.h>
#include <sys/mman.h>
#include <signal.h>
#include <sched.h>
#include <fcntl.h>
#include <stdbool.h>
#include <string.h>

#define STKS	(4*4096)

#ifndef CLONE_NEWPID
#define CLONE_NEWPID    0x20000000
#endif

typedef struct {
	char *log_file;
} opts_t;

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
		fprintf(stderr, "Usage: %s -o/--log-file <log_file>\n", argv[0]);
		return (usage_error != false);
	}

	stk = mmap(NULL, STKS, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON | MAP_GROWSDOWN, 0, 0);
	pid = clone(do_test, stk + STKS, SIGCHLD | CLONE_NEWPID, (void*)&opts);
	if (pid < 0) {
		fprintf(stderr, "clone() failed: %m\n");
		return 1;
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
