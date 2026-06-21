package main

import (
	"encoding/json"
	"fmt"
	"os"

	"tfq/internal/engine"
)

// run returns (stdoutText, exitCode). Kept pure for testing; main wires it to os.
func run(args []string) (string, int) {
	if len(args) < 1 {
		return usage(), 2
	}
	switch args[0] {
	case "inspect":
		if len(args) != 2 {
			return usage(), 2
		}
		fv, err := engine.Inspect(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		b, err := json.MarshalIndent(fv, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return string(b), 0
	default:
		return usage(), 2
	}
}

func usage() string {
	return "usage: tfq inspect <file>"
}

func main() {
	out, code := run(os.Args[1:])
	if out != "" {
		if code == 0 {
			fmt.Println(out)
		} else {
			fmt.Fprintln(os.Stderr, out)
		}
	}
	os.Exit(code)
}
