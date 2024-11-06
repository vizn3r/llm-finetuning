package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/pflag"
	"golang.org/x/net/html"
)

type OutputFormat_t int

const (
	JSON OutputFormat_t = iota
	JSONL
)

var (
	outDir        string
	tempDir       string
	keepTempFiles bool
	datasetName   = "dataset"
	inputLinks    []string
	outFormat     OutputFormat_t

	crawledLinks sync.Map
	semaphore    chan struct{}
)

func SnakeCase(str string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(str)), " ", "_")
}

type PageData struct {
	Heading string `json:"heading"`
	Text    string `json:"text"`
	Titlee  string `json:"title"`
}

func WriteData(title string, data []PageData) error {
	var fName string
	switch outFormat {
	case JSON:
		fName = tempDir + "/" + title + ".json"
	case JSONL:
		fName = tempDir + "/" + title + ".jsonl"
	}

	if _, err := os.Stat(fName); !os.IsNotExist(err) {
		return nil
	}

	f, err := os.Create(fName)
	if err != nil {
		return err
	}
	defer f.Close()
	switch outFormat {
	case JSON:
		encoder := json.NewEncoder(f)
		return encoder.Encode(data)
	case JSONL:
		for _, d := range data {
			jsonData, _ := json.Marshal(d)
			f.Write(jsonData)
			f.Write([]byte("\n"))
		}
	}
	return nil
}

func GetLinks(s *goquery.Selection) []string {
	var links []string
	s.Find("a").Each(func(i int, s *goquery.Selection) {
		link, exists := s.Attr("href")
		if exists && strings.HasPrefix(link, "/") && !strings.HasSuffix(strings.ToLower(link), ".jpg") && !strings.HasSuffix(strings.ToLower(link), ".png") {
			links = append(links, link)
		}
	})
	return links
}

func FormatContentSelection(s *goquery.Selection) (out string) {
	s.Each(func(i int, s *goquery.Selection) {
	})
	return
}

func RemoveDuplicates(arr []string) []string {
	items := make(map[string]any)
	var out []string
	for _, item := range arr {
		if _, exists := items[item]; !exists {
			items[item] = nil
			out = append(out, item)
		}
	}

	return out
}

func Crawl(url string, wg *sync.WaitGroup) {
	defer wg.Done()

	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	// fmt.Println("Crawling", url)

	res, err := http.Get(url)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	defer res.Body.Close()

	rawBody, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}

	reader := strings.NewReader(string(rawBody))
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}

	var pageTitle string
	var data []PageData
	var links []string
	body := doc.Find("body")

	// Get pageTitle
	pageTitle = body.Find("#firstHeading").Find("span").Contents().Text()
	// fmt.Println(pageTitle)

	sectionHeading := pageTitle
	prevHeading := sectionHeading
	var section string
	rawContent := body.Find(".page__main").Find("div.mw-parser-output").Children()
	rawContent.Each(func(i int, s *goquery.Selection) {
		if s.Nodes[0].Type == html.CommentNode || goquery.NodeName(s) == "aside" || goquery.NodeName(s) == "div" || goquery.NodeName(s) == "figure" || goquery.NodeName(s) == "blockquote" {
			return
		}
		if goquery.NodeName(s) == "h1" || goquery.NodeName(s) == "h2" || goquery.NodeName(s) == "h3" || goquery.NodeName(s) == "h4" || goquery.NodeName(s) == "h5" {
			sectionHeading = strings.ReplaceAll(strings.TrimSpace(s.Text()), "[]", "")
		} else {
			section += strings.ReplaceAll(strings.TrimSpace(s.Text()), "\n", "")
		}

		if sectionHeading != prevHeading {
			if section != "" && strings.ToLower(prevHeading) != "see also" {
				data = append(data, PageData{prevHeading, section, pageTitle})
			}
			prevHeading = sectionHeading
			section = ""
		}
	})
	rawContent.Each(func(i int, s *goquery.Selection) {
		links = append(links, GetLinks(s)...)
	})

	links = RemoveDuplicates(links)

	err = WriteData(SnakeCase(strings.ReplaceAll(pageTitle, "/", "_")), data)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}

	baseUrl := strings.TrimPrefix(url, "https://")
	baseUrl = strings.Split(baseUrl, "/")[0]
	baseUrl = "https://" + baseUrl

	for _, link := range links {
		fullLink := baseUrl + link
		if _, crawled := crawledLinks.LoadOrStore(fullLink, true); !crawled {
			// fmt.Println("Found link:", fullLink)
			wg.Add(1)
			go Crawl(baseUrl+link, wg)
		}
	}
}

func CombineJSONFiles() error {
	var combinedData []PageData

	// Read all JSON files from the output directory
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Process only JSON files
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			var items []PageData
			if err := json.Unmarshal(fileContent, &items); err != nil {
				return err
			}

			combinedData = append(combinedData, items...)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error reading files: %v", err)
	}

	// Marshal the combined data back to JSON
	outputFile := filepath.Join(outDir, datasetName+".json")
	output, err := json.MarshalIndent(combinedData, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling combined data: %v", err)
	}

	// Write the combined JSON data to a file
	if err := os.WriteFile(outputFile, output, 0644); err != nil {
		return fmt.Errorf("error writing to output file: %v", err)
	}

	fmt.Println("Combined JSON data written to", outputFile)
	return nil
}

func CleanDir(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	if len(files) > 0 {
		fmt.Println("Cleaning", dir)
	}
	for _, file := range files {
		path := filepath.Join(dir, file.Name())
		err := os.RemoveAll(path)
		if err != nil {
			fmt.Println("Couldn't remove", path)
		}
	}
}

func CombineJSONLFiles() error {
	var combinedData []byte

	// Read all JSON files from the output directory
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Process only JSON files
		if !info.IsDir() && filepath.Ext(path) == ".jsonl" {
			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			combinedData = append(combinedData, fileContent...)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error reading files: %v", err)
	}

	// Marshal the combined data back to JSON
	outputFile := filepath.Join(outDir, datasetName+".jsonl")

	// Write the combined JSON data to a file
	if err := os.WriteFile(outputFile, combinedData, 0644); err != nil {
		return fmt.Errorf("error writing to output file: %v", err)
	}

	fmt.Println("Combined JSONL data written to", outputFile)
	return nil
}

func main() {
	wg := new(sync.WaitGroup)
	semaphore = make(chan struct{}, 50)

	p_inputLinks := pflag.StringArray("links", []string{}, "Links to crawl, use format '--links link1 link2 ...'")
	p_tempDir := pflag.String("temp_dir", "./temp", "Define temporary directory")
	p_outDir := pflag.String("out_dir", "./out", "Output directory for the data")
	p_keepTempFiles := pflag.Bool("keep_temp", false, "Keep temporary directory")
	p_outFormat := pflag.Int("out_format", 0, "Output file format [0 - .json | 1 - .jsonl]")

	pflag.Parse()

	inputLinks = *p_inputLinks
	tempDir = *p_tempDir
	outDir = *p_outDir
	keepTempFiles = *p_keepTempFiles
	outFormat = OutputFormat_t(*p_outFormat)

	if len(inputLinks) == 0 {
		fmt.Println("No links provided, use '--links' or '--help'")
		os.Exit(0)
	}

	// Make dirs
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(outDir, 0755)
	if err != nil {
		panic(err)
	}

	// Clean before use
	CleanDir(tempDir)

	fmt.Println("Output directory:", outDir)
	fmt.Println("Data name:", datasetName)
	fmt.Println("Scaping wiki")

	// Start crawling
	for _, link := range inputLinks {
		if _, crawled := crawledLinks.LoadOrStore(link, true); !crawled && strings.HasPrefix(link, "http") {
			fmt.Println("URL:", link)
			wg.Add(1)
			go Crawl(link, wg)
		}
	}
	wg.Wait()

	switch outFormat {
	case JSON:
		err := CombineJSONFiles()
		if err != nil {
			panic(err)
		}
	case JSONL:
		err := CombineJSONLFiles()
		if err != nil {
			panic(err)
		}
	}

	// Delete temorary files if not keeping
	if !keepTempFiles {
		err := os.RemoveAll(tempDir)
		if err != nil {
			panic(err)
		}
	}
}
