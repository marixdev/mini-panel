package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"mpn/internal/executor"
	"mpn/internal/installer"
)

// detectHost fills ram, cpus and swap from this machine, except for flags the user gave explicitly.
func detectHost(fs *flag.FlagSet, ram, cpus, swap *int) {
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	memMB, swapMB := readMeminfo()
	if !given["ram"] && memMB > 0 {
		*ram = memMB
	}
	if !given["cpus"] {
		*cpus = runtime.NumCPU()
	}
	if !given["swap"] {
		*swap = swapMB
	}
	fmt.Printf("# host: ram=%dMB cpus=%d swap=%dMB\n", *ram, *cpus, *swap)
}

// readMeminfo returns MemTotal and SwapTotal in MB, or zeros when /proc/meminfo is unavailable.
func readMeminfo() (memMB, swapMB int) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		kb, _ := strconv.Atoi(fields[1])
		switch fields[0] {
		case "MemTotal:":
			memMB = kb / 1024
		case "SwapTotal:":
			swapMB = kb / 1024
		}
	}
	return memMB, swapMB
}

// runApply executes the plan on this machine. It refuses to run anywhere but Linux as root.
// selfPath returns the path of the running binary on Linux, or "" elsewhere (a Windows path is no use on a server).
func selfPath() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

func requireLinuxRoot() {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fail(errors.New("-apply needs Linux and root"))
	}
}

func runApply(plan installer.Plan) {
	requireLinuxRoot()
	fail(executor.Apply(plan, &executor.OS{Out: os.Stdout}, os.Stdout))
	fmt.Println("done")
}
