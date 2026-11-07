package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/nvcnvn/dg/internal/pack"
)

const packUsage = `usage: dg pack add <source>[@<version>] [--as <name>] [-C dir]
       dg pack update <name> [<version>] [-C dir]
       dg pack remove <name> [-C dir]
       dg pack install [-C dir]
       dg pack search <text>

Copies packs from git repositories into packs/<name>/, with a pack block in dg.hcl and an entry in
dg.lock. Only dg pack uses the network or needs git on the path; the copy is part of the project.

add      installs a pack. A source is <host>/<path>, fetched over HTTPS, or an https, ssh or file
         URL; //<dir> at its end names a directory of the repository, and a source with no / is
         looked up in the index. The version is a tag or a full commit hash, by default the highest
         vX.Y.Z tag. --as gives the local name, by default the source's last segment. It prints
         the pack's license files.
update   replaces a pack's files, with the same default version.
remove   removes a pack's directory, block and dg.lock entry.
install  writes every pack in dg.lock at its commit, and refuses a pack whose files do not match
         its hash. It changes neither dg.hcl nor dg.lock.
search   lists the index's entries whose name or description holds the text.

add, update and remove then print dg check's diagnostics, leaving the files written, and exit 1 on
errors.
`

// positionals parses args with fl, flags being allowed after the positional arguments, as in
// dg pack add <source> --as <name>, and returns the positional arguments.
func positionals(fl *flag.FlagSet, args []string) ([]string, error) {
	var out []string
	for {
		if err := fl.Parse(args); err != nil {
			return nil, err
		}
		if fl.NArg() == 0 {
			return out, nil
		}
		out = append(out, fl.Arg(0))
		args = fl.Args()[1:]
	}
}

// packCounts gives the least and the most positional arguments of each dg pack command.
var packCounts = map[string][2]int{"add": {1, 1}, "update": {1, 2}, "remove": {1, 1}, "install": {0, 0}, "search": {1, 1}}

// parsePack parses the arguments of a dg pack command, which args[0] names.
func parsePack(args []string, stderr io.Writer) (pos []string, dir, as string, err error) {
	sub := args[0]
	count, ok := packCounts[sub]
	if !ok {
		fmt.Fprint(stderr, packUsage)
		return nil, "", "", fmt.Errorf("pack: unknown command %q", sub)
	}
	fl := flag.NewFlagSet("pack "+sub, flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.StringVar(&dir, "C", ".", "project directory")
	if sub == "add" {
		fl.StringVar(&as, "as", "", "local name of the pack (default the source's last segment)")
	}
	pos, err = positionals(fl, args[1:])
	if err != nil {
		return nil, "", "", err
	}
	if len(pos) < count[0] || len(pos) > count[1] {
		return nil, "", "", fmt.Errorf("pack %s: wrong number of arguments; dg pack --help shows them", sub)
	}
	return pos, dir, as, nil
}

func runPack(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, packUsage)
		return errors.New("pack: name what to do")
	}
	sub := args[0]
	if sub == "-h" || sub == "-help" || sub == "--help" || sub == "help" {
		fmt.Fprint(stdout, packUsage)
		return nil
	}
	pos, dir, as, err := parsePack(args, stderr)
	if err != nil {
		return err
	}
	var in *pack.Installed
	switch sub {
	case "add":
		in, err = pack.Add(dir, pos[0], as)
	case "update":
		in, err = pack.Update(dir, pos[0], append(pos, "")[1])
	case "remove":
		err = pack.Remove(dir, pos[0])
	case "install":
		names, err := pack.Install(dir)
		for _, name := range names {
			fmt.Fprintln(stdout, "installed", name)
		}
		return err
	case "search":
		entries, err := pack.Search(pos[0])
		for _, e := range entries {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", e.Name, e.Source, e.Description)
		}
		return err
	}
	if err != nil {
		return err
	}
	if in != nil {
		for _, file := range in.Licenses {
			fmt.Fprintln(stdout, "license:", file)
		}
		if len(in.Licenses) == 0 {
			fmt.Fprintf(stderr, "warning: the pack %q has no LICENSE or NOTICE file\n", in.Name)
		}
	}
	return runCheck([]string{"-C", dir}, stdout, stderr)
}
