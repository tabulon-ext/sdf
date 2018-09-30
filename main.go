package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var userPath = os.Getenv("HOME")
var sdfPath = userPath + "/.config/sdf"
var baseGit = "git --git-dir=" + sdfPath +
	" --work-tree=" + userPath

// sdf add <path>
// Add files to version control system.
func addToVCS(paths []string) {
	fullCmd := append(strings.Fields(baseGit+" add"), paths...)
	runWithOutput(fullCmd...)
}

// sdf git <valid git command>
// Escape the abstractions! Get full access to the underlying repository.
func delegateCmdToVCS(cmd []string) {
	fullCmd := append(strings.Fields(baseGit), cmd...)
	runWithOutput(fullCmd...)
}

// sdf init <url>
// Initialize the configuration from repository.
func initFromVCS(url string) {
	if _, err := os.Stat(sdfPath); !os.IsNotExist(err) {
		fmt.Println("SDF is already initialized!")
		if !askForConfirmation("Force remove previous configuration?") {
			return
		}
		check(os.RemoveAll(sdfPath))
	}
	// Git magic below
	check(os.MkdirAll(userPath+"/.config", 0600))
	tempDir := userPath + "/.config/sdf-tmp"
	runWithOutput(
		"git", "clone", "--separate-git-dir="+
			sdfPath, url, tempDir,
	)
	// ensure git-modules work.
	modules := tempDir + "/.gitmodules"
	if _, err := os.Stat(modules); !os.IsNotExist(err) {
		check(os.Rename(modules, userPath+"/.gitmodules"))
	}
	check(os.RemoveAll(tempDir))
	gitCmd2 := append(
		strings.Fields(baseGit),
		"config", "status.showUntrackedFiles", "no",
	)
	exec.Command(gitCmd2[0], gitCmd2[1:]...).Run()
	// ensure other users can't see our data.
	check(os.Chmod(sdfPath, 0700))
	fmt.Println("Restored SDF configuration, activate it with 'sdf git checkout .'")
}

// sdf new <url>
// Initialize a new configuration and set default remote URL.
func initNew(url string) {
	if _, err := os.Stat(sdfPath); !os.IsNotExist(err) {
		fmt.Println("SDF is already initialized!")
		if !askForConfirmation("Force remove previous configuration?") {
			return
		}
		check(os.RemoveAll(sdfPath))
	}
	// Git magic below
	exec.Command(
		"git", "init", "--bare",
		sdfPath,
	).Run()
	// This block sets the remote URL
	gitCmd1 := append(
		strings.Fields(baseGit),
		"remote", "add", "master", url,
	)
	exec.Command(gitCmd1[0], gitCmd1[1:]...).Run()
	gitCmd2 := append(
		strings.Fields(baseGit),
		"config", "status.showUntrackedFiles", "no",
	)
	exec.Command(gitCmd2[0], gitCmd2[1:]...).Run()
	// ensure other users can't see our data.
	check(os.Chmod(sdfPath, 0700))
	fmt.Println("Initialized new configuration.")
}

// sdf
// Show current status.
func status() {
	cmd := append(strings.Fields(baseGit), "status")
	runWithOutput(cmd...)
}

// sdf rm <path>
// Remove a file from the repository.
func rmFromVCS(paths []string) {
	fullCmd := append(strings.Fields(baseGit+" rm"), paths...)
	runWithOutput(fullCmd...)
}

// sdf trace <command>
// Launch the given program under strace and then filters
// output to display the files that are opened by it.
func traceCmd(inCmd []string) {
	// test if strace is present
	if _, err := exec.LookPath("strace"); err != nil {
		fmt.Println("Strace not found. Check your $PATH or install it.")
		return
	}
	// test if given binary exist
	if _, err := exec.LookPath(inCmd[0]); err != nil {
		fmt.Println("Binary not executable or doesn't exist. Cannot continue.")
		return
	}
	straceArgs := strings.Fields("-f -e trace=openat")
	fullArgs := append(straceArgs, inCmd...)
	straceCmd := exec.Command("strace")
	straceCmd.Args = append(straceCmd.Args, fullArgs...)
	straceOut, err := straceCmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewReader(straceOut)
	straceCmd.Start()
	uplen := len(userPath) // needed for cleaning output
	for {
		line, err := scanner.ReadString('\n')
		if err == io.EOF {
			break
		}
		temp := strings.Split(line, "\"")
		if len(temp) > 2 { // make sure line has a valid path
			if strings.HasPrefix(temp[1], userPath) { // show stuff from $HOME
				if len(temp[1]) == uplen { // Program viewing home dir; skip
					continue
				}
				fmt.Println(temp[1][uplen+1:]) // remove $HOME prefix
			}
		}
	}
	straceCmd.Wait() // reap process entry from process table
}

func main() {
	if len(os.Args) == 1 {
		status()
		return
	}
	switch os.Args[1] {
	case "add":
		if len(os.Args) < 3 {
			fmt.Println("At least 1 file path is required.")
			return
		}
		addToVCS(os.Args[2:])
	case "git":
		if len(os.Args) < 3 {
			fmt.Println("Please provide more commands.")
			return
		}
		delegateCmdToVCS(os.Args[2:])
	case "init":
		if len(os.Args) >= 4 {
			fmt.Println("Too many parameters.")
			return
		} else if len(os.Args) != 3 {
			fmt.Println("URL required.")
			return
		}
		initFromVCS(os.Args[2])
	case "new":
		if len(os.Args) >= 4 {
			fmt.Println("Too many parameters.")
			return
		} else if len(os.Args) != 3 {
			fmt.Println("URL required.")
			return
		}
		initNew(os.Args[2])
	case "rm":
		if len(os.Args) < 3 {
			fmt.Println("At least 1 file path is required.")
			return
		}
		rmFromVCS(os.Args[2:])
	case "trace":
		if len(os.Args) < 3 {
			fmt.Println("Please provide command.")
			return
		}
		traceCmd(os.Args[2:])
	default:
		fmt.Println("Invalid command.")
		return
	}
}

// askForConfirmation asks the user for confirmation. A user must type in "yes" or "no" and
// then press enter. It has fuzzy matching, so "y", "Y", "yes", "YES", and "Yes" all count as
// confirmations. If the input is not recognized, it will ask again. The function does not return
// until it gets a valid response from the user.
func askForConfirmation(s string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/n]: ", s)

		response, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}

		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}
	}
}

func runWithOutput(cmdStr ...string) {
	cmd := exec.Command(
		cmdStr[0], cmdStr[1:]...,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// https://news.ycombinator.com/item?id=11070797
// Credit where it's due.
