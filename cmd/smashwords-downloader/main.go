package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly"
	"github.com/taylorskalyo/goreader/epub"
)

const (
	smashWordsURL string = "www.smashwords.com"
	localCacheDir string = "/tmp/smashwords_cache"
)

func createBookFileName(title string, textFormat string) string {
	// Remove all non-alphanumeric characters from the title
	re := regexp.MustCompile(`[^\w]`)
	fileName := re.ReplaceAllString(title, "")

	return fmt.Sprintf("%s.%s", fileName, textFormat)
}

func downloadBook(title string, bookLink string, dataDir string, textFormat string) {
	// We can't declare const arrays, so we have to do this
	SUPPORTEDFORMATS := [2]string{"epub", "txt"}

	fileName := createBookFileName(title, textFormat)
	if fileName == "" {
		log.Printf("Skipping %s since the title is all symbols (probably not English)", title)
		return
	}

	filePath := fmt.Sprintf("%s/%s", dataDir, fileName)
	fullUrl := fmt.Sprintf("https://%s%s", smashWordsURL, bookLink)

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0700); err != nil {
			log.Fatal(err)
		}
	}
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal(err)
	}

	// We check if the file already exists before downloading it (including other formats)
	for _, format := range SUPPORTEDFORMATS {
		potentialFilePath := dataDir + "/" + createBookFileName(title, format)
		if _, err := os.Stat(potentialFilePath); err == nil {
			log.Printf("Skipping %s for %s format since it already exists in %s format", title, textFormat, format)
			return
		} else if !os.IsNotExist(err) {
			log.Printf("Error checking if file exists")
		}
	}

	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	resp, err := client.Get(fullUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()

	log.Printf("Downloaded %s to %s\n", title, filePath)
}

func scrapeBookList(pageId int, dataDir string, urlID int, textFormat string) {
	// Create a collector for the page that lists all books
	listCollector := colly.NewCollector(
		colly.AllowedDomains(smashWordsURL),
		colly.CacheDir(localCacheDir),
	)

	// Create another collector to scrape the book pages
	bookCollector := listCollector.Clone()

	// Before making a request print "Visiting ..."
	listCollector.OnRequest(func(r *colly.Request) {
		log.Println("Getting book links from", r.URL.String())
	})

	listCollector.OnError(func(r *colly.Response, err error) {
		log.Println("Request URL:", r.Request.URL, "failed with status code:", r.StatusCode, "Error:", err)
	})

	// Send all the individual book links through the book collector
	listCollector.OnHTML("a[class=library-title]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		bookCollector.Visit(link)
	})

	// Get the text file link and download when available
	bookCollector.OnHTML("div[id=pageContentFull]", func(e *colly.HTMLElement) {
		title := e.ChildText("h1")

		// We check if the book is available in the requested format
		if textFormat == "txt" || textFormat == "all" {
			search := "a[title='Plain text; contains no formatting']"
			e.ForEach(search, func(_ int, e *colly.HTMLElement) {
				book_link := e.Attr("href")
				downloadBook(title, book_link, dataDir, "txt")
			})
		}
		if textFormat == "epub" || textFormat == "all" {
			search := "a[title='Supported by many apps and devices (e.g., Apple Books, Barnes and Noble Nook, Kobo, Google Play, etc.)']"
			e.ForEach(search, func(_ int, e *colly.HTMLElement) {
				book_link := e.Attr("href")
				downloadBook(title, book_link, dataDir, "epub")
			})
		}

	})

	smashwordsCategoryURL := fmt.Sprintf("https://%s/books/category/%d/downloads/0/free/any/%d", smashWordsURL, urlID, pageId)
	listCollector.Visit(smashwordsCategoryURL)
}

func main() {
	// flags used: -url is the url to scrape,
	// -data_dir is the directory to save the files to
	dataDirPtr := flag.String("data_dir", "./data",
		"directory that the book files will download to")

	urlIDPtr := flag.Int("id", 1245,
		"The cooresponding ID for the smashswords url you want to scrape"+
			" (in https://www.smashwords.com/books/category/1245)")

	itemsPerPagePtr := flag.Int("pageitems", 20,
		"The number of items per page on the smashwords list page")

	pagesPtr := flag.Int("pages", 7,
		"The number of pages to scrape")

	textFormatPtr := flag.String("format", "txt",
		"The format of the book to download. Options are 'all', 'txt' or 'epub'"+
			" (default is 'all' for getting all formats avaliable)")

	overwriteSourcePtr := flag.Bool("overwriteSource", true,
		"Save the original file after converting it to the desired format")
	flag.Parse()

	totalBooks := *itemsPerPagePtr * *pagesPtr

	// log the flag parameters out to console
	log.Printf("Scraping %d pages of %d items, (total is %d) each from smashwords url %d.\n", *pagesPtr, *itemsPerPagePtr, totalBooks, *urlIDPtr)
	log.Printf("Selected format is %s.\n", *textFormatPtr)
	log.Printf("Saving files to %s folder.\n", *dataDirPtr)

	// Create a wait group to wait for all the goroutines to finish
	wg := new(sync.WaitGroup)

	// Each list page only shows `bookListSize` books so scrape each one in parallel
	for i := 0; i < (totalBooks); i = i + *itemsPerPagePtr {
		wg.Add(1)
		go func(pageId int) {
			defer wg.Done()
			scrapeBookList(pageId, *dataDirPtr, *urlIDPtr, *textFormatPtr)
		}(i)
	}

	wg.Wait()

	// convert epub to txt if needed
	if *textFormatPtr == "epub" || *textFormatPtr == "all" {
		ConvertEpubGo(*dataDirPtr, *overwriteSourcePtr)
	}
}

// A lot of the actual parsing is done with this repo: https://github.com/taylorskalyo/goreader
func ConvertEpubGo(inputdir string, overwriteSource bool) {
	// get all files in directory
	files, err := os.ReadDir(inputdir)
	if err != nil {
		log.Fatal(err)
	}

	// we time the parsing
	start := time.Now()

	// we count the number of characters
	charCount := 0

	// for each file, if it is an epub, convert it to txt
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".epub") {
			filepath := inputdir + "/" + file.Name()

			// we check if we are being rate limited, if we are,
			// we don't parse the rest of the files (since they will be rate limited too)
			isRateLimited := CheckRateLimit(filepath)
			if isRateLimited {
				log.Fatal("Rate limited by smashwords. Please try again later. (up to 500/24 hours)")
				break
			}

			// We use the goreader library to parse the epub
			rc, err := epub.OpenReader(filepath)
			if err != nil {
				log.Fatal(err)
			}

			// The rootfile (content.opf) lists all of the contents of an epub file.
			// There may be multiple rootfiles, although typically there is only one.
			book := rc.Rootfiles[0]

			// Print book title.
			fmt.Println("Parsing book: ", book.Title, "(file: ", file.Name()+")")

			// stringbuilder to hold the text instead of using goreader's cell system
			var sb strings.Builder

			// generate output file name and file
			outputFileName := strings.TrimSuffix(file.Name(), ".epub") + ".txt"
			outputFilePath := inputdir + "/" + outputFileName
			outputFile, err := os.Create(outputFilePath)
			if err != nil {
				log.Fatal(err)
			}
			defer outputFile.Close()

			// iterate through each chapter in the book
			for _, itemref := range book.Spine.Itemrefs {
				f, err := itemref.Open()
				if err != nil {
					panic(err)
				}

				// parse the chapter into the stringbuilder
				sbret, err := ParseText(f, book.Manifest.Items, sb)
				if err != nil {
					log.Fatal(err)
				}
				// get the string from the stringbuilder
				chapterStr := strings.ReplaceAll(sbret.String(), "	", "")
				charCount += len(chapterStr)

				// writes to file
				outputFile.Write([]byte(chapterStr))

				// Close the itemref.
				f.Close()

				// clear the stringbuilder
				sb.Reset()

			}

			//if overwriteSource is true, delete the original epub file
			if overwriteSource {
				err = os.Remove(filepath)
				if err != nil {
					log.Fatal(err)
				}
			}

			// Close the rootfile.
			rc.Close()

		}

	}
	if charCount > 0 {
		elapsed := time.Since(start)
		fmt.Printf("Parsing took %s, parsed %d characters at a rate of %d characters per second.\n", elapsed, charCount, int(float64(charCount)/elapsed.Seconds()))
	}
}

// We check if we are being rate limited on epub files by scanning the epub downloaded for a string, returns true if we are being rate limited
func CheckRateLimit(inputdir string) bool {
	searchstring := "We are currently throttling downloads for users who download more than 500 per day,"

	//we get the one epub file in the directory
	file, err := os.Open(inputdir)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	//we also check if the file is empty
	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}
	if fileInfo.Size() == 0 {
		log.Printf("File is empty")
		return true
	}

	// we read the file
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), searchstring) {
			return true
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return false
}
