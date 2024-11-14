#include <stdlib.h>
#include <stdio.h>
#include <string.h>

static void usage(char *argv0)
{
	fprintf(stdout, "%s - [ -s character][ -h character] TEMPLATE FILE\n", argv0);
	exit(1);
}

int main(int argc, char **argv)
{
	if (argc < 3)
		usage(argv[0]);

	const char delim = '#';
	const char special = '\0';

	FILE *tmpl = fopen(argv[1], "r");
	FILE *src  = fopen(argv[2], "r");
	if (!tmpl || !src)
		usage(argv[0]);

	int nmap = 10;
	char **smap = calloc(sizeof(*smap), nmap);
	/*char **smap = NULL;*/

	char *line = "#@HEADER\n";
	size_t size = 0;
	long lno = 0;
	do {
		if (lno >= nmap) {
			nmap *= 2;
			if (!(smap = realloc(smap, nmap * sizeof(*smap)))) {
				fprintf(stdout, "realloc fail\n");
				exit(1);
			}
		}
		if (*line == delim) {
			smap[lno] = strdup(line);
		} else {
			smap[lno] = smap[lno-1];
		}
		lno++;
	} while (getline(&line, &size, src) > 0);

	for (int i = 0; i < nmap; i++)
		fprintf(stdout, "%d - %s\n", i, smap[i]);

	fprintf(stdout, "exit\n");

	return EXIT_SUCCESS;
}
