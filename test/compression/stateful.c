// SPDX-License-Identifier: Apache-2.0
#define _GNU_SOURCE
#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/wait.h>
#include <unistd.h>

struct child_info {
	pid_t pid;
	unsigned long address;
	size_t size;
};

static volatile sig_atomic_t change_state, stopping;

static void notify(int signal)
{
	if (signal == SIGUSR1)
		change_state = 1;
	else
		stopping = 1;
}

static void fail(const char *operation)
{
	perror(operation);
	exit(1);
}

static void run_child(int ready, size_t page)
{
	struct child_info child;
	unsigned char *data;
	size_t size = 2 * page;
	sigset_t empty;

	if (!freopen("/dev/null", "w", stdout)) {
		perror("redirect child stdout");
		_exit(1);
	}

	data = mmap(NULL, size, PROT_READ | PROT_WRITE,
		    MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
	if (data == MAP_FAILED) {
		perror("map child memory");
		_exit(1);
	}
	if (prctl(PR_SET_NAME, "child")) {
		perror("set child name");
		_exit(1);
	}

	memset(data, 'K', size);
	memcpy(data + page - 8, "CHILD_PRIVATE_MARKER", 20);
	memcpy(data + size - 16, "END_CHILD_MEMORY", 16);

	memset(&child, 0, sizeof(child));
	child.pid = getpid();
	child.address = (unsigned long)data;
	child.size = size;
	if (write(ready, &child, sizeof(child)) != sizeof(child))
		_exit(1);
	close(ready);

	sigemptyset(&empty);
	while (!stopping)
		sigsuspend(&empty);
	_exit(0);
}

static struct child_info spawn_child(size_t page)
{
	struct child_info child;
	ssize_t received;
	char unexpected;
	int ready[2];
	pid_t pid;

	if (pipe(ready))
		fail("pipe");

	pid = fork();
	if (pid < 0)
		fail("fork");
	if (pid == 0) {
		close(ready[0]);
		run_child(ready[1], page);
	}

	close(ready[1]);
	received = read(ready[0], &child, sizeof(child));
	if (received < 0)
		fail("read child readiness");
	if (received != sizeof(child)) {
		fprintf(stderr, "child readiness: incomplete message (%zd of %zu bytes)\n",
			received, sizeof(child));
		exit(1);
	}

	received = read(ready[0], &unexpected, 1);
	if (received < 0)
		fail("read child readiness EOF");
	if (received != 0) {
		fprintf(stderr, "child readiness: unexpected data after readiness message\n");
		exit(1);
	}
	close(ready[0]);
	return child;
}

static void stop_child(pid_t pid)
{
	if (kill(pid, SIGTERM) && errno != ESRCH)
		fail("kill child");
	if (waitpid(pid, NULL, 0) < 0)
		fail("wait child");
}

static void manifest(const char *directory, const char *phase, struct child_info child)
{
	char path[4096], temporary[4096];
	FILE *file;

	if (snprintf(path, sizeof(path), "%s/state.json", directory) >= (int)sizeof(path) ||
	    snprintf(temporary, sizeof(temporary), "%s/state.tmp", directory) >= (int)sizeof(temporary))
		fail("manifest path");

	file = fopen(temporary, "w");
	if (!file)
		fail("manifest open");

	fprintf(file, "{\"phase\":\"%s\",\"root\":%d,\"child\":%d,"
		"\"child_address\":%lu,\"child_size\":%zu}\n",
		phase, getpid(), child.pid, child.address, child.size);
	if (fclose(file) || rename(temporary, path))
		fail("manifest commit");
}

int main(int argc, char **argv)
{
	struct child_info child;
	sigset_t blocked, empty;
	size_t page;
	int transitioned = 0;

	if (argc != 3 || strlen(argv[2]) != 6)
		return 1;
	page = (size_t)sysconf(_SC_PAGESIZE);

	if (signal(SIGUSR1, notify) == SIG_ERR)
		fail("set state-change handler");
	if (signal(SIGTERM, notify) == SIG_ERR)
		fail("set termination handler");

	sigemptyset(&blocked);
	sigaddset(&blocked, SIGUSR1);
	sigaddset(&blocked, SIGTERM);
	sigemptyset(&empty);
	if (sigprocmask(SIG_BLOCK, &blocked, NULL))
		fail("block signals");

	child = spawn_child(page);
	manifest(argv[1], "before", child);

	printf("ready\n");
	fflush(stdout);
	if (!freopen("/dev/null", "w", stdout))
		fail("stdio redirect");

	while (!stopping) {
		if (change_state && !transitioned) {
			char *state = getenv("CHECKPOINTCTL_STATE");
			if (!state || strlen(state) != 6)
				fail("state environment");

			memcpy(state, "after!", 6);
			memcpy(argv[2], "after!", 6);
			manifest(argv[1], "after", child);
			transitioned = 1;
		}
		sigsuspend(&empty);
	}

	stop_child(child.pid);
	return 0;
}
