#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#define die(msg) fprintf(stderr, "%s: " msg "\n", argv[0])
#define USAGE "[ -s char ][ -h char ] TEMPLATE FILE"

static char delim;
static char special;

static char **mapsecs(FILE *src, unsigned int *mapsize)
{
	int    size   = 10;
	char **secmap = malloc(sizeof(*secmap) * size);

	char  *line = strdup("#@HEADER\n");
	size_t llen = 0;
	unsigned long lno = 0;
	do {
		if (lno >= size) {
			size *= 2;
			if (!(secmap = realloc(secmap, size * sizeof(*secmap))))
				return NULL;
		}
		secmap[lno++] = (*line == delim) ? strdup(line) : NULL;
	} while (getline(&line, &llen, src) > 0);
	free(line);
	line = NULL;

	*mapsize = size;
	return secmap;
}

static void putsec(char *sec, FILE *src, char **secmap, unsigned int size)
{
	int target = 0;
	while (target < size && secmap[target] && strcmp(sec, secmap[target++]));

	size_t llen = 0;
	char  *line = NULL;
	unsigned long lno = 0;
	while (getline(&line, &llen, src) > 0) {

	}
}

static void subst(FILE *tmpl, FILE *src, char **secmap, unsigned int size)
{
	size_t llen = 0;
	char *line = NULL;
	while (getline(&line, &llen, tmpl) > 0) {
		line[llen-1] = '\0';
		if (*line != delim) {
			puts(line);
		} else {
			putsec(line, src, secmap, size);
		}
	}
}

int main(int argc, char **argv)
{
	if (argc < 3)
		goto usage;

	/* TODO: PARSE OPTS */
	delim = '#';
	special = '\0';

	FILE *tmpl = fopen(argv[1], "r");
	FILE *src  = fopen(argv[2], "r");
	if (!tmpl || !src)
		goto usage;

	unsigned int size = 0;
	char **secmap = mapsecs(src, &size);
	if (!secmap)
		goto errmem;

	subst(tmpl, src, secmap, size);

	return EXIT_SUCCESS;

errfile:
	die("files couldn't be opened");
	goto usage;
errmem:
	die("out of memory");
	fclose(tmpl);
	fclose(src);
usage:
	die(USAGE);
	return EXIT_FAILURE;
}
