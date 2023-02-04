package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	termbox "github.com/nsf/termbox-go"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/gocolly/colly"
	"github.com/taylorskalyo/goreader/epub"
)

const (
	smashWordsURL    string = "www.smashwords.com"
	localCacheDir    string = "/tmp/smashwords_cache"
	westernRomanceID int    = 1245
	maxPageId        int    = 140
	bookListSize     int    = 20 // Number of books on each smashwords list page
)

//parser is a part of the goreader repo for parsing epubs
type parser struct {
	tagStack  []atom.Atom
	tokenizer *html.Tokenizer
	doc       cellbuf
	items     []epub.Item
	sb        strings.Builder
}

//cellbuf is a part of the goreader repo for parsing epubs
type cellbuf struct {
	cells   []termbox.Cell
	width   int
	lmargin int
	col     int
	row     int
	space   bool
	fg, bg  termbox.Attribute
}

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

	overwriteSourcePtr := flag.Bool("overwriteSource", true,
		"Save the original file after converting it to the desired format")
	flag.Parse()

	totalBooks := *itemsPerPagePtr * *pagesPtr

	//log the flag parameters out to console
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

	//convert epub to txt if needed
	if *textFormatPtr == "epub" {

		ConvertEpubGo(*dataDirPtr, *overwriteSourcePtr)
	}
}

//A lot of the actual parsing is done with this repo: https://github.com/taylorskalyo/goreader
func ConvertEpubGo(inputdir string, overwriteSource bool) {
	//get all files in directory
	files, err := ioutil.ReadDir(inputdir)
	if err != nil {
		log.Fatal(err)
	}

	//we time the parsing
	start := time.Now()

	//we count the number of characters
	charCount := 0

	//for each file, if it is an epub, convert it to txt
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".epub") {
			filepath := inputdir + "/" + file.Name()

			//We use the goreader library to parse the epub
			rc, err := epub.OpenReader(filepath)
			if err != nil {
				panic(err)
			}
			defer rc.Close()
			// The rootfile (content.opf) lists all of the contents of an epub file.
			// There may be multiple rootfiles, although typically there is only one.
			book := rc.Rootfiles[0]

			// Print book title.
			fmt.Println("Parsing book: ", book.Title, "(file: ", file.Name()+")")

			//stringbuilder to hold the text instead of using goreader's cell system
			var sb strings.Builder

			//generate output file name and file
			outputFileName := strings.TrimSuffix(file.Name(), ".epub") + ".txt"
			outputFilePath := inputdir + "/" + outputFileName
			outputFile, err := os.Create(outputFilePath)
			if err != nil {
				log.Fatal(err)
			}
			defer outputFile.Close()

			//iterate through each chapter in the book
			for _, itemref := range book.Spine.Itemrefs {
				f, err := itemref.Open()
				if err != nil {
					panic(err)
				}

				//parse the chapter into the stringbuilder
				sbret, err := parseText(f, book.Manifest.Items, sb)
				if err != nil {
					log.Fatal(err)
				}
				//get the string from the stringbuilder
				chapterStr := strings.ReplaceAll(sbret.String(), "	", "")
				charCount += len(chapterStr)

				//writes to file
				outputFile.Write([]byte(chapterStr))

				// Close the itemref.
				f.Close()

				//clear the stringbuilder
				sb.Reset()
			}

			//if overwriteSource is true, delete the original epub file
			if overwriteSource {
				err = os.Remove(filepath)
				if err != nil {
					log.Fatal(err)
				}
			}

		}

	}
	if charCount > 0 {
		elapsed := time.Since(start)
		fmt.Printf("Parsing took %s, parsed %d characters at a rate of %d characters per second.\n", elapsed, charCount, int(float64(charCount)/elapsed.Seconds()))
	}
}

// parseText takes in html content via an io.Reader and returns a buffer
// containing only plain text.
func parseText(r io.Reader, items []epub.Item, sb strings.Builder) (strings.Builder, error) {
	tokenizer := html.NewTokenizer(r)
	doc := cellbuf{width: 80}
	p := parser{tokenizer: tokenizer, doc: doc, items: items, sb: sb}
	err := p.parse(r)
	if err != nil {
		return p.sb, err
	}
	return p.sb, nil
}

// parse walks an html document and renders elements to a cell buffer document.
func (p *parser) parse(io.Reader) (err error) {
	for {
		tokenType := p.tokenizer.Next()
		token := p.tokenizer.Token()
		switch tokenType {
		case html.ErrorToken:
			err = p.tokenizer.Err()
		case html.StartTagToken:
			p.tagStack = append(p.tagStack, token.DataAtom) // push element
			fallthrough
		case html.SelfClosingTagToken:
			p.handleStartTag(token)
		case html.TextToken:
			p.handleText(token)
		case html.EndTagToken:
			p.tagStack = p.tagStack[:len(p.tagStack)-1] // pop element
		}
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// handleText appends text elements to the parser buffer. It filters elements
// that should not be displayed as text (e.g. style blocks).
func (p *parser) handleText(token html.Token) {
	// Skip style tags
	if len(p.tagStack) > 0 && p.tagStack[len(p.tagStack)-1] == atom.Style {
		return
	}
	p.doc.style(p.tagStack)
	//I think the appendText is needed to properly parse the tags
	p.doc.appendText(string(token.Data))
	p.sb.WriteString(string(token.Data))

}

// handleStartTag appends text representations of non-text elements (e.g. image alt
// tags) to the parser buffer.
func (p *parser) handleStartTag(token html.Token) {
	switch token.DataAtom {
	case atom.Img:
		// Display alt text in place of images.
		for _, a := range token.Attr {
			switch atom.Lookup([]byte(a.Key)) {
			case atom.Alt:
				text := fmt.Sprintf("Alt text: %s", a.Val)
				p.doc.appendText(text)
				p.doc.row++
				p.doc.col = p.doc.lmargin
			case atom.Src:
				for _, item := range p.items {
					if item.HREF == a.Val {

						break
					}
				}
			}
		}
	case atom.Br:
		p.doc.row++
		p.doc.col = p.doc.lmargin
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Title,
		atom.Div, atom.Tr:
		p.doc.row += 2
		p.doc.col = p.doc.lmargin
	case atom.P:
		p.doc.row += 2
		p.doc.col = p.doc.lmargin
		p.doc.col += 2
	case atom.Hr:
		p.doc.row++
		p.doc.col = 0
		p.doc.appendText(strings.Repeat("-", p.doc.width))
	}
}

// style sets the foreground/background attributes for future cells in the cell
// buffer document based on HTML tags in the tag stack.
func (c *cellbuf) style(tags []atom.Atom) {
	fg := termbox.ColorDefault
	for _, tag := range tags {
		switch tag {
		case atom.B, atom.Strong, atom.Em:
			fg |= termbox.AttrBold
		case atom.I:
			fg |= termbox.ColorYellow
		case atom.Title:
			fg |= termbox.ColorRed
		case atom.H1:
			fg |= termbox.ColorMagenta
		case atom.H2:
			fg |= termbox.ColorBlue
		case atom.H3, atom.H4, atom.H5, atom.H6:
			fg |= termbox.ColorCyan
		}
	}
	c.fg = fg
}

// appendText appends text to the cell buffer document.
func (c *cellbuf) appendText(str string) {
	if len(str) <= 0 {
		return
	}
	if c.col < c.lmargin {
		c.col = c.lmargin
	}
	runes := []rune(str)
	/*
		if unicode.IsSpace(runes[0]) {
			c.space = true
		}*/
	scanner := bufio.NewScanner(strings.NewReader(str))
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		if c.col != c.lmargin && c.space {
			c.col++
		}
		word := []rune(scanner.Text())
		if len(word) > c.width-c.col {
			c.row++
			c.col = c.lmargin
		}
		for _, r := range word {
			c.setCell(c.col, c.row, r, c.fg, c.bg)
			c.col++
		}
		//c.space = true
	}
	if !unicode.IsSpace(runes[len(runes)-1]) {
		//c.space = false
	}
}

// setCell changes a cell's attributes in the cell buffer document at the given
// position.
func (c *cellbuf) setCell(x, y int, ch rune, fg, bg termbox.Attribute) {
	// Grow in steps of 1024 when out of space.
	for y*c.width+x >= len(c.cells) {
		c.cells = append(c.cells, make([]termbox.Cell, 1024)...)
	}
	c.cells[y*c.width+x] = termbox.Cell{Ch: ch, Fg: fg, Bg: bg}
}
