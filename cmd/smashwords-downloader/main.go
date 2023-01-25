package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sync"

	"github.com/gocolly/colly"
)

const (
	smashWordsURL    string = "www.smashwords.com"
	localCacheDir    string = "/tmp/smashwords_cache"
	westernRomanceID int    = 1245
	maxPageId        int    = 140
	bookListSize     int    = 20 // Number of books on each smashwords list page
)

func createBookFileName(title string, textFormat string) string {
	// Remove all non-alphanumeric characters from the title
	re := regexp.MustCompile(`[^\w]`)
	fileName := re.ReplaceAllString(title, "")

	return fmt.Sprintf("%s.%s", fileName, textFormat)
}

func downloadBook(title string, bookLink string, dataDir string, textFormat string) {
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
		search := "a[title='Plain text; contains no formatting']"
		if textFormat == "epub" {
			search = "a[title='Supported by many apps and devices (e.g., Apple Books, Barnes and Noble Nook, Kobo, Google Play, etc.)']"
		}

		e.ForEach(search, func(_ int, e *colly.HTMLElement) {
			book_link := e.Attr("href")
			downloadBook(title, book_link, dataDir, textFormat)
		})
	})

	smashwordsCategoryURL := fmt.Sprintf("https://%s/books/category/%d/downloads/0/free/any/%d", smashWordsURL, urlID, pageId)
	listCollector.Visit(smashwordsCategoryURL)
}

func main() {
	//flags used: -url is the url to scrape,
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
		"The format of the book to download. Options are 'txt' or 'epub'")
	flag.Parse()

	totalBooks := *itemsPerPagePtr * *pagesPtr

	//log the flag parameters out to console
	log.Printf("Scraping %d pages of %d items, (total is %d) each from smashwords url %d.\n", *pagesPtr, *itemsPerPagePtr, totalBooks, *urlIDPtr)
	log.Printf("Selected format is %s.\n", *textFormatPtr)
	log.Printf("Saving files to %s.\n", *dataDirPtr)

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
}
