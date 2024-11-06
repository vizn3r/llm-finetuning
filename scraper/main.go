package main

import (
	"data/wiki"
	"fmt"
	"os"
	"strings"
)

type Command struct {
	Call string
	Desc string
	Run  func(args []string)
}

var Commands []Command

func PrintCommandsHelp() {
	fmt.Println("Avaliable commands:")
	for _, command := range Commands {
		fmt.Print("    ", command.Call, " - ", command.Desc, "\n")
	}
	fmt.Println("\nTo see usage of a command, run 'scraper [command] --help'")
}

func main() {
	Commands = append(Commands, Command{
		Call: "wiki",
		Desc: "Scape wiki page",
		Run:  wiki.WikiScrape,
	})

	if len(os.Args) == 1 {
		PrintCommandsHelp()
		os.Exit(0)
	}

	call := os.Args[1]

	for _, command := range Commands {
		if strings.ToLower(call) == command.Call {
			command.Run(os.Args[2:])
		}
	}
}
