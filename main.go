package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	// Define flags
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")
	flag.BoolVar(verboseFlag, "v", false, "Enable verbose output (shorthand)")
	timingsFlag := flag.Bool("timings", false, "Enable timing output")
	flag.BoolVar(timingsFlag, "t", false, "Enable timing output (shorthand)")
	// Parse flags
	flag.Parse()
	args := flag.Args()
	if len(args) > 0 {
		input := strings.Join(args, ",")
		HandleInput(input, *verboseFlag, *timingsFlag)

		return
	} else {
		RunInteractive(*verboseFlag, *timingsFlag)
	}
}

func RunInteractive(verbose bool, timings bool) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("For help, type ':h' or ':help' (not implemented yet)\n")
		fmt.Print("Enter a word (or exit with ':q' or ':quit'): ")
		input, error := reader.ReadString('\n')
		trimmedInput := strings.TrimSpace(input)

		// Handle any read error
		if error != nil {
			fmt.Println("Error reading input:", error)
			continue
		}

		// Handle exit commands first
		if trimmedInput == ":q" || trimmedInput == ":quit" {
			fmt.Print("Exiting...\n")
			break
		} else if trimmedInput == ":hist" || trimmedInput == ":history" {
			history, err := getRecentSearches()
			if err == nil {
				fmt.Printf("History: %s\n", history)
			} else {
				fmt.Println("Unexpected JSON/filepath error:", error)
			}
		} else {
			HandleInput(trimmedInput, verbose, timings)
		}
	}
}

func HandleInput(input string, verbose bool, timingsFlag bool) {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		fmt.Println("No input provided.")
		return
	}

	_, err := addRecentSearch(input)
	if err != nil {
		fmt.Printf("Unexpected JSON or filepath error: %v\n", err)
	}

	fmt.Printf("Handling input: %s\n", input)

	entries, timings, err := LookupAll(input)
	if err != nil {
		fmt.Printf("Error looking up word: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Println("No entries found.")
		return
	}

	// Merge all meanings by part of speech
	grouped := make(map[string]map[string][]string)
	for _, entry := range entries {
		if grouped[entry.Source] == nil {
			grouped[entry.Source] = make(map[string][]string)
		}
		grouped[entry.Source][entry.PartOfSpeech] = append(grouped[entry.Source][entry.PartOfSpeech], formatDefinition(entry))
	}

	// Print grouped results
	for source, posGroup := range grouped {
		if timingsFlag {
			fmt.Printf("\n=== Source: %s (%v) ===\n", source, timings.Sources[source].Round(100*time.Microsecond))
		} else {
			fmt.Printf("\n=== Source: %s ===\n", source)
		}

		for partOfSpeech, defs := range posGroup {
			fmt.Printf("\n[%s]\n", strings.Title(partOfSpeech))
			for i, def := range defs {
				if !verbose && i >= 3 { // limit to 3 per part of speech in non-verbose mode
					break
				}
				fmt.Printf("  • %s\n", def)
			}
		}
	}
	if timingsFlag {
		fmt.Printf("\nTotal lookup time: %v\n", timings.Total.Round(100*time.Microsecond))
	}
	fmt.Println("--------------------------")
}

func formatDefinition(entry Definition) string {
	defStr := entry.Definition
	if entry.Example != "" {
		defStr += fmt.Sprintf(" (e.g., \"%s\")", entry.Example)
	}
	return defStr
}
