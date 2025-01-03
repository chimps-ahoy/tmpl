#include <stdbool.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#define die(msg) fprintf(stderr, "%s: %s\n", argv[0], msg)
#define USAGE "[ -s char ][ -h char ] TEMPLATE FILE"

#define MIN(x,y) (((x)<(y))?(x):(y))

#define ENTIRE_FILE "@CONTENT"
#define PREAMBLE "@HEADER"
static char delim;
static char tokdelims[5] = { 0, ' ', '\t', '\n' };
static char special;

static void putf(FILE *f)
{
	char *s = NULL;
	size_t l = 0;
	getdelim(&s, &l, EOF, f);
	fputs(s, stdout);
	rewind(f);
}

/* IDEA: putsec and subst coroutines?? */
static void putsec(char *sec, size_t slen, FILE *src)
{
	if (!strncmp(sec, ENTIRE_FILE, MIN(slen, sizeof(ENTIRE_FILE)))) {
		putf(src);
		return;
	}

	char * line = strdup("#"PREAMBLE);
	size_t llen = sizeof("#"PREAMBLE);
	int    endl = llen + 1;
	bool print = false;
	do {
		line[endl-1] = '\0';
		if (print && *line != delim)
			puts(line);
		else if (*line == delim)
			print = !strncmp(line+1, sec, MIN(slen, llen-1));
	} while ((endl = getline(&line, &llen, src)) > 0);
	rewind(src);
}

static void subst(FILE *tmpl, FILE *src)
{
	int endl;
	char *line = NULL;
	size_t llen = 0;
	while ((endl = getline(&line, &llen, tmpl)) > 0) {
		char *strt = line + strspn(line, tokdelims+1);
		if (*strt == delim)
			putsec(strtok(strt, tokdelims), llen, src);
		else
			fputs(line, stdout);
	}
}

int main(int argc, char **argv)
{
	if (argc < 3)
		goto usage;

	/* TODO: PARSE OPTS */
	delim = '#';
	tokdelims[0] = delim;
	special = '\0';

	FILE *tmpl = fopen(argv[1], "r");
	FILE *src  = fopen(argv[2], "r");
	if (!tmpl || !src)
		goto errfile;

	subst(tmpl, src);

	fclose(tmpl);
	fclose(src);
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
