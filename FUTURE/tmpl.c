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
	char * line = strdup("#@HEADER\n");
	size_t llen = sizeof("#@HEADER\n");;
	int    endl = 9;
	bool print = false;
	do {
		line[endl-1] = '\0';
		if (print && *line != delim)
			puts(line);
		else if (*line == delim)
			print = !strncmp(line, sec, MIN(slen, llen));
	} while ((endl = getline(&line, &llen, src)) > 0);
	rewind(src);
}

static void putf(FILE *f)
{
	char c;
	while ((c = getc(f)) != EOF) putc(c, stdout);
	rewind(f);
}

/* TODO: strtok */
static void subst(FILE *tmpl, FILE *src)
{
	int endl;
	char *line = NULL;
	size_t llen = 0;
	while ((endl = getline(&line, &llen, tmpl)) > 0) {
		line[endl-1] = '\0';
		if (*line != delim)
			puts(line);
		else if (!strncmp(line, "#@CONTENT", llen))
			putf(src);
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
