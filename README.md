# dataset-downloader <!-- omit in toc -->

Contains code that build into docker images that can be used to download datasets for training machine learning models.

Contents:
- [smashwords-downloader](#smashwords-downloader)

## smashwords-downloader

This script downloads plain text files of Western Romance books publicaly avaible on [Smashwords](https://www.smashwords.com/). This website has been used to create popular Machine Learning datasets like [BookCorpus](https://huggingface.co/datasets/bookcorpus).

The source code located in `cmd/smashwords-downloader`. 
It can be built into an executable with the command `go build -o main *.go`.

The `main.go` script takes the following arugments:
```
  -data_dir string
        directory that the book files will download to (default "./data")
  
  -id integer
        The cooresponding ID for the smashswords url you want to scrape
        https://www.smashwords.com/books/category/1105/downloads/0/free would have an ID of 1105 (default is 1245 == western romance)

  -pageitems integer
        The number of items smashword has per page, shouldn't need to be changed. (default is 20)

  -pages integer
        The number of pages you want to download. (default is 7)

  -format string
        The format of text you want to download, some books only have limited format avaliability.
        (default is all for .txt and .epub files), options are (all, txt, epub). Note: Not all books have all formats.
        You may get significantly less books downloaded then specified based on file format.

  -overwriteSource bool
        If you are downloading in a format other then txt (ex. EPUB), set this to true if you
        don't want to keep the source files, and just want to keep the .txt files (default true)
```

Example Execution

Download Western Romance novels in .txt format to directory data
>./main -data_dir data

Download Adventure novels to directory data, downloading 20 items (don't change this), 10 pages (total 200 items) in epub format, converting to text and overwriting source folder

> ./main -data_dir data -id 1105 -pageitems 20 -pages 10 -format epub -overwriteSource=true

