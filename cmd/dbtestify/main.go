package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/alecthomas/kong"
	"github.com/fatih/color"
	"github.com/joho/godotenv"

	"github.com/shibukawa/dbtestify"
	"github.com/shibukawa/dbtestify/connect"
	"github.com/shibukawa/dbtestify/httpapi/serve"
)

var deleteTaskC = color.New(color.FgHiRed).SprintFunc()
var insertTaskC = color.New(color.FgHiBlue).SprintFunc()
var nameC = color.New(color.FgBlue, color.Bold).SprintFunc()
var okC = color.New(color.FgGreen).SprintFunc()
var errC = color.New(color.FgRed).SprintFunc()
var infoC = color.New(color.FgYellow).SprintFunc()

var cli struct {
	DB      string `flag:"" env:"DBTESTIFY_CONN" help:"Database connection setting"`
	Quiet   bool   `flag:"" short:"q"`
	Verbose bool   `flag:"" short:"v"`

	Seed struct {
		//Gen         string   `short:"g" enum:"playwright,cypress,go," default:""`
		IncludeTag []string `flag:"" short:"i" optional:"Tag name that is used for filtering data (for include)."`
		ExcludeTag []string `flag:"" short:"e" optional:"Tag name that is used for filtering data (for exclude)."`
		BatchSize  int      `flag:"" short:"b" default:"50"`
		Truncates  []string `flag:"" short:"t" help:"Truncate table target before seeding."`
		SourceFile string   `arg:"" type:"existingfile" help:"Data set file to import"`
		Targets    []string `arg:"" optional:"" help:"Target table (default: all tables in source file)"`
	} `cmd:"" help:"Seeding database content for testing"`

	Assert struct {
		//Gen         string   `short:"g" enum:"playwright,cypress,go," default:""`
		IncludeTag []string `flag:"" short:"i" optional:"Tag name that is used for filtering data (for include)."`
		ExcludeTag []string `flag:"" short:"e" optional:"Tag name that is used for filtering data (for exclude)."`
		SourceFile string   `arg:"" type:"existingfile"`
		Targets    []string `arg:"" optional:"" help:"Target table (default: all tables in source file)"`
	} `cmd:""`

	Http struct {
		Port uint16 `flag:"" short:"p" default:"8000"`
		Dir  string `arg:"" type:"existingdir"`
	} `cmd:""`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, errC(".env load error: %s\n"), errC(err.Error()))
		os.Exit(1)
	}

	kctx := kong.Parse(&cli)
	switch kctx.Command() {
	case "seed <source-file>":
		if cli.DB == "" {
			fmt.Fprintln(os.Stderr, errC("--db=<src> or DBTESTIFY_CONN envvar is required to specify database location."))
			os.Exit(1)
		}
		dbc, err := connect.NewDBConnector(ctx, cli.DB)
		if err != nil {
			fmt.Fprintf(os.Stderr, errC("database location is invalid: %s\n"), errC(err.Error()))
			os.Exit(1)
		}
		f, err := os.Open(cli.Seed.SourceFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, errC("can't read source file: %s\n"), errC(err.Error()))
			os.Exit(1)
		}
		defer f.Close()
		data, err := dbtestify.ParseYAML(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, errC("data set file load error: %s\n"), errC(err.Error()))
			os.Exit(1)
		}

		var startTime time.Time
		opt := dbtestify.SeedOpt{
			BatchSize:    cli.Seed.BatchSize,
			Operations:   data.Operation,
			IncludeTags:  cli.Seed.IncludeTag,
			ExcludeTags:  cli.Seed.ExcludeTag,
			TargetTables: cli.Seed.Targets,
			Callback: func(targetTable, task string, start bool, err error) {
				if cli.Quiet {
					return
				}
				switch task {
				case "truncate":
					if start {
						startTime = time.Now()
						fmt.Printf("%s: '%s' ...", deleteTaskC("truncating"), nameC(targetTable))
					} else if err != nil {
						fmt.Printf(" %s\n    %s\n", errC("NG"), errC(err.Error()))
					} else {
						fmt.Printf(" %s (%s)\n", okC("OK"), infoC(time.Since(startTime)))
					}
				case "insert":
					if start {
						startTime = time.Now()
						fmt.Printf("%s: '%s' ...", insertTaskC("importing"), nameC(targetTable))
					} else if err != nil {
						fmt.Printf(" %s\n    %s\n", errC("NG"), errC(err.Error()))
					} else {
						fmt.Printf(" %s (%s)\n", okC("OK"), infoC(time.Since(startTime)))
					}
				case "upsert":
					if start {
						startTime = time.Now()
						fmt.Printf("%s: '%s' ...", insertTaskC("upserting"), nameC(targetTable))
					} else if err != nil {
						fmt.Printf(" %s\n    %s\n", errC("NG"), errC(err.Error()))
					} else {
						fmt.Printf(" %s (%s)\n", okC("OK"), infoC(time.Since(startTime)))
					}
				case "delete":
					if start {
						startTime = time.Now()
						fmt.Printf("%s: '%s' ...", deleteTaskC("deleting"), nameC(targetTable))
					} else if err != nil {
						fmt.Printf(" %s\n    %s\n", errC("NG"), errC(err.Error()))
					} else {
						fmt.Printf(" %s (%s)\n", okC("OK"), infoC(time.Since(startTime)))
					}
				default:
					panic(task)
				}
			},
		}
		for _, t := range cli.Seed.Truncates {
			opt.Operations[t] = dbtestify.TruncateOperation
		}
		err = dbtestify.Seed(ctx, dbc, data, opt)
		if err != nil {
			fmt.Fprintf(os.Stderr, errC("seed error: %s\n"), errC(err.Error()))
			os.Exit(1)
		}
	case "assert <source-file>":
		if cli.DB == "" {
			fmt.Fprintln(os.Stderr, errC("--db=<src> or dbtestify_CONN envvar is required to specify database location."))
			os.Exit(1)
		}
		dbc, err := connect.NewDBConnector(ctx, cli.DB)
		if err != nil {
			fmt.Fprintf(os.Stderr, errC("database location is invalid: %s\n"), errC(err.Error()))
			os.Exit(1)
		}
		f, err := os.Open(cli.Assert.SourceFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, errC("can't read source file: %s\n"), errC(err.Error()))
			os.Exit(1)
		}
		defer f.Close()
		data, err := dbtestify.ParseYAML(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, errC("data set file load error: %s\n"), err.Error())
			os.Exit(1)
		}
		var startTime time.Time
		ok, _, err := dbtestify.Assert(ctx, dbc, data, dbtestify.AssertOpt{
			IncludeTags:  cli.Assert.IncludeTag,
			ExcludeTags:  cli.Assert.ExcludeTag,
			TargetTables: cli.Assert.Targets,
			Callback: func(targetTable string, s dbtestify.MatchStrategy, start bool, err error) {
				if cli.Quiet {
					return
				}
				if start {
					startTime = time.Now()
					fmt.Printf("%s: '%s' ...", deleteTaskC("fetching data"), nameC(targetTable))
				} else if err != nil {
					fmt.Printf(" %s\n    %s\n", errC("NG"), errC(err.Error()))
				} else {
					fmt.Printf(" %s (%s) (match: %s)\n", okC("OK"), infoC(time.Since(startTime)), s.String())
				}
				startTime = time.Now()
			},
			DiffCallback: dbtestify.DumpDiffCLICallback(false, cli.Quiet),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed error: %s\n", err.Error())
			os.Exit(1)
		}

		if !ok {
			fmt.Println(errC("Not Match"))
			os.Exit(1)
		} else {
			fmt.Println(okC("Match"))
		}
	case "http <dir>":
		if cli.DB == "" {
			fmt.Fprintln(os.Stderr, errC("--db=<src> or DBTESTIFY_CONN envvar is required to specify database location."))
			os.Exit(1)
		}
		err := serve.Start(ctx, cli.Http.Dir, cli.DB, cli.Http.Port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "server start error: %s\n", err.Error())
			os.Exit(1)
		}
	}
}
