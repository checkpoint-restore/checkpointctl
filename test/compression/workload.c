// SPDX-License-Identifier: Apache-2.0
#define _GNU_SOURCE
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

/* Odd-sized adjacent VMAs force short blocks at protection boundaries.
 * A long compressible region is followed by incompressible and zero pages.
 * Keep the Go expectations independent of this C workload.
 */
enum {
	SHORT_VMA_END = 13,
	SECOND_VMA_END = 47,
	COMPRESSIBLE_END = 1153,
	RANDOM_END = 1282,
	MAPPING_PAGES = 1536,
	SENTINEL_START = MAPPING_PAGES - 1,
};

int main(void)
{
	size_t page = (size_t)sysconf(_SC_PAGESIZE);
	size_t size = MAPPING_PAGES * page;
	/* Keep the sentinel separate so the preceding VMA has all-zero blocks. */
	size_t boundaries[] = {
		0, SHORT_VMA_END, SECOND_VMA_END, COMPRESSIBLE_END,
		RANDOM_END, SENTINEL_START, MAPPING_PAGES,
	};
	uint32_t random = 0x12345678;
	unsigned char *data;

	data = mmap(NULL, size, PROT_READ | PROT_WRITE,
		    MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
	if (data == MAP_FAILED)
		return 1;

	for (size_t i = 0; i < size; i++) {
		if (i < COMPRESSIBLE_END * page)
			data[i] = 'Z';
		else if (i < RANDOM_END * page) {
			random ^= random << 13;
			random ^= random >> 17;
			random ^= random << 5;
			data[i] = (unsigned char)random;
		} else
			data[i] = 0; /* Fault in every zero page. */
	}

	/* The final marker exposes a truncated tail in compact memory output. */
	memcpy(data + size - 16, "END_OF_MAPPING!!", 16);

	for (size_t i = 0; i + 1 < sizeof(boundaries) / sizeof(boundaries[0]); i++) {
		int prot = i % 2 ? PROT_READ | PROT_WRITE : PROT_READ;
		if (mprotect(data + boundaries[i] * page,
			     (boundaries[i + 1] - boundaries[i]) * page, prot))
			return 1;
	}

	/* EOF on stdout confirms setup is complete before creating a checkpoint. */
	printf("%lu %zu\n", (unsigned long)data, page);
	fflush(stdout);
	if (!freopen("/dev/null", "w", stdout))
		return 1;

	for (;;)
		pause();
}
