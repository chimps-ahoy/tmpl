#include <stdbool.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#define die(msg) fprintf(stderr, "%s: " msg "\n", argv[0])
#define USAGE "[ -s char ][ -h char ] TEMPLATE FILE"

#define MIN(x,y) (((x)<(y))?(x):(y))

static char delim;
static char special;

/* IDEA: putsec and subst coroutines?? */
static void putsec(char *sec, size_t slen, FILE *src)
{
	size_t llen = 0;
	int endl;
	char *line = NULL;
	bool print = false;
	while ((endl = getline(&line, &llen, src)) > 0) {
		line[endl-1] = '\0';
		if (print && *line != delim)
			puts(line);
		else if (*line == delim)
			print = !strncmp(line, sec, MIN(slen, llen));
	}
	rewind(src);
}

/* TODO: strtok */
static void subst(FILE *tmpl, FILE *src)
{
	size_t llen = 0;
	int endl;
	char *line = NULL;
	while ((endl = getline(&line, &llen, tmpl)) > 0) {
		line[endl-1] = '\0';
		if (*line != delim)
			puts(line);
		else
			putsec(line, llen, src);
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
		goto errfile;

	subst(tmpl, src);
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
