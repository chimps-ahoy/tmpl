// Package main is a command-lint tool `zs` called Zen Static for generating static websites
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
	"time"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yuin/goldmark"
	// "github.com/yuin/goldmark-meta"
	// "github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/wikilink"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

const (
	// ZSDIR is the default directory for storing layouts and extensions
	ZSDIR = ".zs"

	// ZSCONFIG is the default configuration name (without extension)
	ZSCONFIG = "config"

	// ZSIGNORE is the default ignore file
	ZSIGNORE = ".zsignore"

	// PUBDIR is the default directory for publishing final built content
	PUBDIR = ".pub"

	// DefaultIgnore is the default set of ignore patterns if no .zsignore
	DefaultIgnore = `*~
*.bak
.*

COPYING
LICENSE
Makefile
README.md`
)

// Ignore holds a set of patterns to ignore from parsing a .zsignore file
// var Ignore *ignore.GitIgnore

// Parser holds a configured global instance of the goldmark markdown parser
var Parser goldmark.Markdown

var (
	configFile        string
	enabledExtensions []string
)

// Extensions is a mapping of name to extension and the default set of extensions enabled
// which can be overridden with -e/--extension or the extensions key
// in ia config file such as .zs/config.yml
var Extensions = map[string]goldmark.Extender{
	"highlighting": highlighting.NewHighlighting(
		highlighting.WithStyle("github"),
	),
	"wikilink": &wikilink.Extender{},
}

// Vars holds a map of global variables
type Vars map[string]string

// MapKeys returns a slice of keys from a map
func MapKeys[K comparable, V any](m map[K]V) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}

// NewTicker is a function that wraps a time.Ticker and ticks immediately instead of waiting for the first interval
func NewTicker(d time.Duration) *time.Ticker {
	ticker := time.NewTicker(d)
	oc := ticker.C
	nc := make(chan time.Time, 1)
	go func() {
		nc <- time.Now()
		for tm := range oc {
			nc <- tm
		}
	}()
	ticker.C = nc
	return ticker
}

// RootCmd is the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:     "zs",
	Short:   "zs the zen static site generator",
	Long: `zs is an extremely minimal static site generator written in Go.

  - Keep your texts in markdown, or HTML format right in the main directory of your blog/site.
  - Keep all service files (extensions, layout pages, deployment scripts etc) in the .zs subdirectory.
  - Define variables in the header of the content files using YAML front matter:
  - Use placeholders for variables and plugins in your markdown or html files, e.g. {{ title }} or {{ command arg1 arg2 }}.
  - Write extensions in any language you like and put them into the .zs sub-directory.
  - Everything the extensions prints to stdout becomes the value of the placeholder.
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var extensions []goldmark.Extender
		for _, name := range viper.GetStringSlice("extensions") {
			if extender, valid := Extensions[name]; valid {
				extensions = append(extensions, extender)
			} else {
				log.Printf("invalid extension: %s", name)
			}
		}

		Parser = goldmark.New(
			goldmark.WithExtensions(extensions...),
			goldmark.WithParserOptions(
				parser.WithAttribute(),
				parser.WithAutoHeadingID(),
			),
			goldmark.WithRendererOptions(
				html.WithXHTML(),
				html.WithUnsafe(),
			),
		)

		return nil
	},
}

// BuildCmd is the build sub-command that performs whole builds or single builds
var BuildCmd = &cobra.Command{
	Use:   "build [<file>]",
	Short: "Builds the whole site or a single file",
	Long:  `The build command builds the entire site or a single file if specified.`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			ctx := context.Background()
			if err := buildAll(ctx, false); err != nil {
				_, err := fmt.Printf("error building site: %w", err); return err
			}
			return nil
		}

		if err := build(args[0], os.Stdout, globals()); err != nil {
			_, err := fmt.Printf("error building file %q", args[0]); return err
		}

		return nil
	},
}

// GenerateCmd is the generate sub-command that builds partial fragments
var GenerateCmd = &cobra.Command{
	Use:     "generate",
	Aliases: []string{"gen"},
	Short:   "Generates partial fragments",
	Long:    `The generate command parses and renders partial fragments from stdin and writes to stdout`,
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := generate(os.Stdin, os.Stdout, globals()); err != nil {
			_, err := fmt.Printf("error generating fragment: %w", err); return err
		}

		return nil
	},
}

// ServeCmd is the serve sub-command that performs whole builds or single builds
var ServeCmd = &cobra.Command{
	Use:   "serve [flags]",
	Short: "Serves the site and rebuilds automatically",
	Long:  `The serve command serves the site and watches for rebuilds automatically`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var wg errgroup.Group

		_, err := cmd.Flags().GetString("bind")
		if err != nil {
			_, err := fmt.Printf("error getting bind flag: %w", err); return err
		}

		_, err = cmd.Flags().GetString("root")
		if err != nil {
			_, err := fmt.Printf("error getting root flag: %w", err); return err
		}

		if err := wg.Wait(); err != nil {
			_, err := fmt.Printf("error running serve: %w", err); return err
		}

		return nil
	},
}

// VarCmd is the var sub-command that performs whole builds or single builds
var VarCmd = &cobra.Command{
	Use:     "var <file> [<var>...]",
	Aliases: []string{"vars"},
	Short:   "Display variables for the specified file",
	Long: `The var command extracts and display sll teh variables defined in a file.
If the name of variables (optional) are passed as additional arguments, only those variables
are display instead of all variables (the default behaviors).`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := ""

		vars, _, err := getVars(args[0], globals())
		if err != nil {
			_, err := fmt.Printf("error getting variables from %s: %w", args[0], err); return err
		}

		if len(args) > 1 {
			for _, a := range args[1:] {
				s = s + vars[a] + "\n"
			}
		} else {
			for k, v := range vars {
				s = s + k + ":" + v + "\n"
			}
		}
		fmt.Println(strings.TrimSpace(s))

		return nil
	},
}

// WatchCmd is the watch sub-command that performs whole builds or single builds
var WatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watches for file changes and rebuilds modified files",
	Long:  `The watch command watches for any changes to files and rebuilds them automatically`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var wg errgroup.Group

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		wg.Go(func() error {
			if err := buildAll(ctx, true); err != nil {
				_, err := fmt.Printf("error watching for changes: %w", err); return err
			}
			return nil
		})

		if err := wg.Wait(); err != nil {
			_, err := fmt.Printf("error running watch: %w", err); return err
		}

		return nil
	},
}

// renameExt renames extension (if any) from oldext to newext
// If oldext is an empty string - extension is extracted automatically.
// If path has no extension - new extension is appended
func renameExt(path, oldext, newext string) string {
	if oldext == "" {
		oldext = filepath.Ext(path)
	}
	if oldext == "" || strings.HasSuffix(path, oldext) {
		return strings.TrimSuffix(path, oldext) + newext
	}
	return path
}

// globals returns list of global OS environment variables that start
// with ZS_ prefix as Vars, so the values can be used inside templates
func globals() Vars {
	vars := Vars{
		"title":       viper.GetString("title"),
		"description": viper.GetString("description"),
		"keywords":    viper.GetString("keywords"),
	}

	if viper.GetBool("production") {
		vars["production"] = "1"
	}

	// Variables from the environment in the form of ZS_<name>=<value>
	for _, e := range os.Environ() {
		pair := strings.Split(e, "=")
		if strings.HasPrefix(pair[0], "ZS_") {
			vars[strings.ToLower(pair[0][3:])] = pair[1]
		}
	}

	// Variables from the command-line -v/--vars (or env var as $ZS_VARS) or configuration
	// Note: These will override the previous variables if names clash.
	for _, e := range viper.GetStringSlice("vars") {
		pair := strings.Split(e, "=")
		vars[pair[0]] = pair[1]
	}

	return vars
}

// run executes a command or a script. Vars define the command environment,
// each zs var is converted into OS environment variable with ZS_ prefix
// prepended.  Additional variable $ZS contains path to the zs binary. Command
// stderr is printed to zs stderr, command output is returned as a string.
func run(vars Vars, cmd string, args ...string) (string, error) {
	// First check if partial exists (.html)
	if b, err := ioutil.ReadFile(filepath.Join(ZSDIR, cmd+".html")); err == nil {
		return string(b), nil
	}

	var errbuf, outbuf bytes.Buffer
	c := exec.Command(cmd, args...)
	env := []string{"ZS=" + os.Args[0], "ZS_OUTDIR=" + PUBDIR}
	env = append(env, os.Environ()...)
	for k, v := range vars {
		if k != "content" {
			env = append(env, "ZS_"+strings.ToUpper(k)+"="+v)
		}
	}
	c.Env = env
	c.Stdout = &outbuf
	c.Stderr = &errbuf

	if err := c.Run(); err != nil {
		log.Printf("error running command: %s", cmd)
		log.Print(errbuf.String())
		return "", err
	}

	return string(outbuf.Bytes()), nil
}

// getVars returns list of variables defined in a text file and actual file
// content following the variables declaration. Header is separated from
// content by an empty line. Header can be either YAML or JSON.
// If no empty newline is found - file is treated as content-only.
func getVars(path string, globals Vars) (Vars, string, error) {
	// if Ignore.MatchesPath(path) {
	// 	return nil, "", nil
	// }

	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("error getting vars from %q: %w", path, err)
		return nil, "", errors.New("358")
	}
	s := string(b)

	// Pick some default values for content-dependent variables
	v := Vars{}
	title := strings.Replace(strings.Replace(path, "_", " ", -1), "-", " ", -1)
	v["title"] = strings.ToTitle(title)
	v["description"] = ""
	v["file"] = path
	v["url"] = path[:len(path)-len(filepath.Ext(path))] + ".html"
	v["output"] = filepath.Join(PUBDIR, v["url"])

	// Override default values with globals
	for name, value := range globals {
		v[name] = value
	}

	// Add layout if none is specified
	if _, ok := v["layout"]; !ok {
		v["layout"] = "layout.html"
	}

	delim := "\n---\n"
	sep := strings.Index(s, delim)
	if sep == -1 {
		return v, s, nil
	}

	header := s[:sep]
	body := s[sep+len(delim):]

	vars := Vars{}
	if err := yaml.Unmarshal([]byte(header), &vars); err != nil {
		log.Printf("%e", err)
		log.Print("failed to parse header")
		return v, s, nil
	}
	// Override default values + globals with the ones defines in the file
	for key, value := range vars {
		v[key] = value
	}
	if strings.HasPrefix(v["url"], "./") {
		v["url"] = v["url"][2:]
	}
	return v, body, nil
}

// Render expanding zs plugins and variables
func render(s string, vars Vars) (string, error) {
	openingDelimiter := viper.GetString("opening-delim")
	closingDelimiter := viper.GetString("closing-delim")

	out := &bytes.Buffer{}
	for {
		from := strings.Index(s, openingDelimiter)
		if from == -1 {
			out.WriteString(s)
			return out.String(), nil
		}

		to := strings.Index(s, closingDelimiter)
		if to == -1 {
			_, err := fmt.Printf("closing delimiter not found")
			return "", err
		}

		out.WriteString(s[:from])
		cmd := s[from+len(openingDelimiter) : to]
		s = s[to+len(closingDelimiter):]
		m := strings.Fields(strings.TrimSpace(cmd))
		if len(m) == 1 {
			if v, ok := vars[m[0]]; ok {
				out.WriteString(v)
				continue
			}
		}
		if _, err := exec.LookPath(m[0]); err == nil {
			if res, err := run(vars, m[0], m[1:]...); err == nil {
				out.WriteString(res)
			} else {
				log.Printf("%e: error running command: %s", err, m[0])
			}
		} else {
			if !viper.GetBool("production") {
				out.WriteString(fmt.Sprintf("%s: plugin or variable not found", m[0]))
			}
		}
	}

}

// Renders markdown with the given layout into html expanding all the macros
func buildMarkdown(path string, w io.Writer, vars Vars) error {
	v, body, err := getVars(path, vars)
	if err != nil {
		return err
	}

	source, err := render(body, v)
	if err != nil {
		return err
	}
	v["source"] = source

	buf := &bytes.Buffer{}
	if err := Parser.Convert([]byte(source), buf); err != nil {
		return err
	}
	v["content"] = buf.String()

	if w == nil {
		out, err := os.Create(filepath.Join(PUBDIR, renameExt(path, "", ".html")))
		if err != nil {
			return err
		}
		defer out.Close()
		w = out
	}

	return buildHTML(filepath.Join(ZSDIR, v["layout"]), w, v)
}

// Renders text file expanding all variable macros inside it
func buildHTML(path string, w io.Writer, vars Vars) error {
	v, body, err := getVars(path, vars)
	if err != nil {
		return err
	}
	if body, err = render(body, v); err != nil {
		return err
	}
	tmpl, err := template.New("").Delims("<%", "%>").Parse(body)
	if err != nil {
		return err
	}
	if w == nil {
		f, err := os.Create(filepath.Join(PUBDIR, path))
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return tmpl.Execute(w, vars)
}

// Copies file as is from path to writer
func buildRaw(path string, w io.Writer) error {
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	if w == nil {
		stat, err := os.Stat(path)
		if err != nil {
			return err
		}

		fn := filepath.Join(PUBDIR, path)

		out, err := os.Create(fn)
		if err != nil {
			return err
		}
		defer out.Close()

		if err := os.Chmod(fn, stat.Mode()); err != nil {
			return err
		}

		w = out
	}

	if _, err := io.Copy(w, r); err != nil {
		return err
	}

	return nil
}

func build(path string, w io.Writer, vars Vars) error {
	// if Ignore.MatchesPath(path) {
	// 	return nil
	// }

	ext := filepath.Ext(path)
	if ext == ".md" || ext == ".mkd" {
		return buildMarkdown(path, w, vars)
	} else if ext == ".html" || ext == ".xml" {
		return buildHTML(path, w, vars)
	}
	return buildRaw(path, w)
}

func buildAll(ctx context.Context, watch bool) error {
	ticker := NewTicker(time.Second)
	defer ticker.Stop()

	lastModified := time.Unix(0, 0)
	modified := false

	vars := globals()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			os.Mkdir(PUBDIR, 0755)
			err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
				// rebuild if changes to .zs/ or .zsignore
				if (filepath.Base(path) == ZSIGNORE || filepath.Dir(path) == ZSDIR) && info.ModTime().After(lastModified) {
					if filepath.Base(path) == ZSIGNORE {
						// Ignore = ParseIgnoreFile(path)
					}
					// reset lastModified to 0 so everything rebuilds
					lastModified = time.Unix(0, 0)
					return nil
				}

				// ignore hidden files and directories and ignored patterns
				// if filepath.Base(path)[0] == '.' || strings.HasPrefix(path, ".") || Ignore.MatchesPath(path) {
				// 	return nil
				// }

				// inform user about fs walk errors, but continue iteration
				if err != nil {
					log.Printf("%e: error walking directory", err)
					return nil
				}

				if info.IsDir() {
					os.Mkdir(filepath.Join(PUBDIR, path), 0755)
					return nil
				} else if info.ModTime().After(lastModified) {
					if !modified {
						// First file in this build cycle is about to be modified
						if _, err := exec.LookPath("prehook"); err == nil {
							if _, err := run(vars, "prehook"); err != nil {
								log.Printf("%e: error running prehook", err)
							}
							modified = true
						}
					}
					log.Printf("build: %s", path)
					return build(path, nil, vars)
				}
				return nil
			})
			if modified {
				// At least one file in this build cycle has been modified
				if _, err := exec.LookPath("posthook"); err == nil {
					if _, err := run(vars, "posthook"); err != nil {
						log.Printf("%e: error running posthook", err)
					}
					modified = false
				}
			}
			if !watch {
				return err
			}
			lastModified = time.Now()
		}
	}
}

// gen generates partial fragments
func generate(r io.Reader, w io.Writer, v Vars) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	body := string(data)

	source, err := render(body, v)
	if err != nil {
		return err
	}

	if err := Parser.Convert([]byte(source), w); err != nil {
		return err
	}

	return nil
}

func ensureFirstPath(p string) {
	paths := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	if len(paths) > 0 && paths[0] != p {
		paths = append([]string{p}, paths...)
		os.Setenv("PATH", strings.Join(paths, string(os.PathListSeparator)))
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().BoolP("debug", "D", false, "enable debug logging $($ZS_DEBUG)")
	RootCmd.PersistentFlags().StringVarP(&configFile, "config", "C", "", "config file (default: .zs/config.yml)")

	RootCmd.PersistentFlags().StringSliceP("extensions", "e", MapKeys(Extensions), "override and enable specific extensions")
	RootCmd.PersistentFlags().BoolP("production", "p", false, "enable production mode ($ZS_PRODUCTION)")
	RootCmd.PersistentFlags().StringP("title", "t", "", "site title ($ZS_TITLE)")
	RootCmd.PersistentFlags().StringP("description", "d", "", "site description ($ZS_DESCRIPTION)")
	RootCmd.PersistentFlags().StringP("keywords", "k", "", "site keywords ($ZS_KEYWORDS)")
	RootCmd.PersistentFlags().StringSliceP("vars", "v", nil, "additional variables")

	RootCmd.PersistentFlags().StringP("opening-delim", "o", "{{", "opening delimiter for plugins")
	RootCmd.PersistentFlags().StringP("closing-delim", "c", "{{", "closing delimiter for plugins")

	viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))
	viper.SetDefault("debug", false)

	viper.BindPFlag("extensions", RootCmd.PersistentFlags().Lookup("extensions"))
	viper.SetDefault("extensions", MapKeys(Extensions))

	viper.BindPFlag("production", RootCmd.PersistentFlags().Lookup("production"))
	viper.SetDefault("production", false)

	viper.BindPFlag("title", RootCmd.PersistentFlags().Lookup("title"))
	viper.SetDefault("title", "")

	viper.BindPFlag("description", RootCmd.PersistentFlags().Lookup("description"))
	viper.SetDefault("description", "")

	viper.BindPFlag("keywords", RootCmd.PersistentFlags().Lookup("keywords"))
	viper.SetDefault("keywords", "")

	viper.BindPFlag("vars", RootCmd.PersistentFlags().Lookup("vars"))
	viper.SetDefault("vars", "")

	viper.BindPFlag("opening-delim", RootCmd.PersistentFlags().Lookup("opening-delim"))
	viper.SetDefault("opening-delim", "{{")
	viper.BindPFlag("closing-delim", RootCmd.PersistentFlags().Lookup("closing-delim"))
	viper.SetDefault("closing-delim", "}}")

	ServeCmd.Flags().StringP("bind", "b", ":8000", "set the [<address>]:<port> to listen on")
	ServeCmd.Flags().StringP("root", "r", PUBDIR, "set the root directory to serve")

	RootCmd.AddCommand(BuildCmd)
	RootCmd.AddCommand(GenerateCmd)
	RootCmd.AddCommand(ServeCmd)
	RootCmd.AddCommand(VarCmd)
	RootCmd.AddCommand(WatchCmd)

	// prepend .zs to $PATH, so plugins will be found before OS commands
	w, _ := os.Getwd()
	ensureFirstPath(filepath.Join(w, ZSDIR))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if configFile == "" {
		// Use config file from .zs/config.yml
		viper.AddConfigPath(ZSDIR)
		viper.SetConfigName(ZSCONFIG)
	} else {
		// Use config file from the flag.
		viper.SetConfigFile(configFile)
	}

	// from the environment
	viper.SetEnvPrefix("ZS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Printf("%e: error reading config %s (using defaults)", err, viper.ConfigFileUsed())
		}
	}
}

// ParseIgnoreFile parsers a .zsignore file or uses the default if an error occurred
// func ParseIgnoreFile(fn string) *ignore.GitIgnore {
// 	obj, err := ignore.CompileIgnoreFile(ZSIGNORE)
// 	if err != nil {
// 		if !errors.Is(err, os.ErrNotExist) {
// 			log.Printf(err).Printf("error parsing .zsignore: %s (using defaults)s", fn)
// 		}
// 		return ignore.CompileIgnoreLines(DefaultIgnore)
// 	}
//
// 	return obj
// }

func main() {
	// prepend .zs to $PATH, so plugins will be found before OS commands
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("%e: error getting current working directory", err)
	}
	ensureFirstPath(filepath.Join(cwd, ZSDIR))

	// initializes Ignore (.zsignore) patterns
	// Ignore = ParseIgnoreFile(ZSIGNORE)

	if err := RootCmd.Execute(); err != nil {
		log.Printf("%e: error executing command", err)
		os.Exit(1)
	}
}
