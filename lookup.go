package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Common structure for all API responses
type Definition struct {
	Word         string
	PartOfSpeech string
	Definition   string
	Example      string
	Source       string
}

type DictEntry struct { // dictionaryapi.dev response structure
	Word     string `json:"word"`
	Meanings []struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Example    string   `json:"example,omitempty"`
			Synonyms   []string `json:"synonyms,omitempty"`
			Antonyms   []string `json:"antonyms,omitempty"`
		} `json:"definitions"`
	} `json:"meanings"`
}

type SparqlEntry struct { // DBnary response structure
	Head struct {
		Vars []string `json:"vars"`
	} `json:"head"`
	Results struct {
		Bindings []map[string]map[string]string `json:"bindings"`
	} `json:"results"`
}

type LookupTiming struct {
	Total   time.Duration
	Sources map[string]time.Duration // e.g., "dictionaryapi.dev": 150ms, "DBnary": 300ms
}

func LookupAll(word string) ([]Definition, LookupTiming, error) {
	/*
		Lookup the word in multiple dictionary APIs concurrently
		Returns a combined list of type Definition from all sources
		If flag is true, also returns timing information, else -1
	*/
	startTimeTotal := time.Now()

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	defChan := make(chan []Definition, 2)
	errChan := make(chan error, 2)
	durChan := make(chan struct {
		name string
		dur  time.Duration
	}, 2)

	// DictionaryAPI.dev goroutine
	go func() {
		defer waitGroup.Done()
		start := time.Now()
		entries, err := LookupDictEntry(word)
		if err != nil {
			errChan <- err
			return
		}
		defChan <- ConvertDictEntries(entries)
		durChan <- struct {
			name string
			dur  time.Duration
		}{name: "dictionaryapi.dev", dur: time.Since(start)}
	}()

	go func() {
		defer waitGroup.Done()
		start := time.Now()
		entries, err := LookupDBnaryEntry(word)
		if err != nil {
			errChan <- err
			return
		}
		defChan <- ConvertDBnaryEntries(entries, word)
		durChan <- struct {
			name string
			dur  time.Duration
		}{name: "DBnary", dur: time.Since(start)}
	}()

	go func() {
		waitGroup.Wait()
		close(defChan)
		close(errChan)
		close(durChan)
	}()

	var allDefinitions []Definition
	for defs := range defChan {
		allDefinitions = append(allDefinitions, defs...)
	}

	timings := LookupTiming{
		Total:   time.Since(startTimeTotal),
		Sources: make(map[string]time.Duration),
	}
	for d := range durChan {
		timings.Sources[d.name] = d.dur
	}

	// Check if any errors occurred
	if len(errChan) > 0 {
		return allDefinitions, LookupTiming{}, <-errChan
	}

	return allDefinitions, timings, nil
}

func LookupDictEntry(word string) ([]DictEntry, error) {
	/*
		Fetch a dictionary entry from dictionaryapi.dev
		Expects JSON array as response
	*/
	apiURL := "https://api.dictionaryapi.dev/api/v2/entries/en/" + url.PathEscape(word)
	client := &http.Client{Timeout: 10 * time.Second}

	response, error := client.Get(apiURL)
	if error != nil {
		return nil, error
	}
	defer response.Body.Close() // Ensure the body is closed after reading or on crash etc.

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: received status code %d", response.StatusCode)
	}

	// Decode the JSON response
	decoder := json.NewDecoder(response.Body)
	token, err := decoder.Token() // Peek at the first token, expecting '['
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return nil, fmt.Errorf("JSON error: Expected array but got something else")
	}

	var entries []DictEntry
	for decoder.More() {
		var entry DictEntry
		if err := decoder.Decode(&entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	_, err = decoder.Token() // Consume the closing ]¨
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func LookupDBnaryEntry(word string) ([]SparqlEntry, error) {
	endpoint := "http://kaiko.getalp.org/sparql"
	query := fmt.Sprintf(`
	PREFIX ontolex: <http://www.w3.org/ns/lemon/ontolex#>
	PREFIX lexinfo: <http://www.lexinfo.net/ontology/2.0/lexinfo#>
	PREFIX dbnary: <http://kaiko.getalp.org/dbnary#>
	PREFIX skos:   <http://www.w3.org/2004/02/skos/core#>
	PREFIX rdf:    <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
	
	SELECT ?definition ?partOfSpeech ?example
	WHERE {
	  ?entry ontolex:canonicalForm/ontolex:writtenRep "%s"@en ;
	         ontolex:sense ?sense .
	
	  OPTIONAL { ?entry lexinfo:partOfSpeech ?partOfSpeech . }
	
	  # Definitions can be literals OR blank nodes; if it's a node, pull the literal from it.
	  OPTIONAL {
	    ?sense (skos:definition|dbnary:definition|ontolex:definition) ?defNode .
	    OPTIONAL { ?defNode (rdf:value|skos:definition) ?defLit . }
	    BIND( IF(isLiteral(?defNode), ?defNode, ?defLit) AS ?definition )
	  }
	
	  # Examples are usually literals, but handle node case similarly (most data won't need this).
	  OPTIONAL {
	    ?sense (dbnary:example|ontolex:usage) ?exNode .
	    OPTIONAL { ?exNode rdf:value ?exLit . }
	    BIND( IF(isLiteral(?exNode), ?exNode, ?exLit) AS ?example )
	  }
	
	  # Optional: prefer English when there is a language tag
	  FILTER( !BOUND(?definition) || lang(?definition) = "" || langMatches(lang(?definition), "en") )
	  FILTER( !BOUND(?example)    || lang(?example)    = "" || langMatches(lang(?example),    "en") )
	}
	LIMIT 50
    `, word)

	params := url.Values{}
	params.Add("query", query)

	apiURL := endpoint + "?" + params.Encode()
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", "gofetch/0.1 (+local)")

	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close() // Ensure the body is closed after reading or on crash etc.

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: received status code %d", response.StatusCode)
	}

	var result SparqlEntry
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}

	return []SparqlEntry{result}, nil
}

func ConvertDictEntries(entries []DictEntry) []Definition {
	var definitions []Definition
	for _, entry := range entries {
		for _, meaning := range entry.Meanings {
			for _, def := range meaning.Definitions {
				definitions = append(definitions, Definition{
					Word:         entry.Word,
					PartOfSpeech: meaning.PartOfSpeech,
					Definition:   def.Definition,
					Example:      def.Example,
					Source:       "dictionaryapi.dev",
				})
			}
		}
	}
	return definitions
}

func ConvertDBnaryEntries(entries []SparqlEntry, word string) []Definition {
	var definitions []Definition
	for _, entry := range entries {
		for _, binding := range entry.Results.Bindings {
			def := ""
			if v, ok := binding["definition"]; ok {
				def = v["value"]
			}
			if strings.TrimSpace(def) == "" {
				continue
			}

			pos := ""
			if v, ok := binding["partOfSpeech"]; ok {
				pos = v["value"]
				if i := strings.LastIndex(pos, "#"); i != -1 && i+1 < len(pos) {
					pos = pos[i+1:]
				}
			}

			ex := ""
			if v, ok := binding["example"]; ok {
				ex = v["value"]
			}

			definitions = append(definitions, Definition{
				Word:         word,
				PartOfSpeech: pos,
				Definition:   def,
				Example:      ex,
				Source:       "DBnary",
			})
		}
	}

	return definitions
}
