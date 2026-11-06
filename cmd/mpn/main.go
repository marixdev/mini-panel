package main

import (
	"flag"
	"fmt"
	"os"

	"mpn/internal/tune"
)

const version = "0.0.1-dev"

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mpn <version|tune|plan|site|db|backup|root|web> [flags]")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version)
	case "tune":
		runTune(os.Args[2:])
	case "plan":
		runPlan(os.Args[2:])
	case "site":
		runSite(os.Args[2:])
	case "db":
		runDB(os.Args[2:])
	case "backup":
		runBackup(os.Args[2:])
	case "root":
		runRoot(os.Args[2:])
	case "web":
		runWeb(os.Args[2:])
	default:
		usage()
	}
}

// runTune prints the generated settings for a host of the given size, without changing anything.
func runTune(args []string) {
	fs := flag.NewFlagSet("tune", flag.ExitOnError)
	ram := fs.Int("ram", 1024, "total RAM in MB")
	cpus := fs.Int("cpus", 1, "CPU count")
	engine := fs.String("engine", "mariadb", "database engine: mariadb or mysql")
	_ = fs.Parse(args)

	p := tune.New(tune.Input{RAMMB: *ram, CPUs: *cpus})
	conf, err := p.DBConf(*engine)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Printf("# tier=%s swap=%dMB journal=%dMB nginx_workers=%d worker_connections=%d\n\n",
		p.Tier, p.SwapMB, p.JournalMaxMB, p.NginxWorkers, p.WorkerConnections)
	fmt.Println(conf)
	fmt.Println(p.PHPPool("example.com", "example", "/run/php/example.sock"))
	fmt.Print(p.OpcacheINI())
}
