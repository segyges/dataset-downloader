# dataset-downloader <!-- omit in toc -->

Contains code that build into docker images that can be used to download datasets for training machine learning models.

Contents:
- [smashwords-downloader](#smashwords-downloader)

## smashwords-downloader

This script downloads plain text files of Western Romance books publicaly avaible on [Smashworks](https://www.smashwords.com/). This website has been used to create popular Machine Learning datasets like [BookCorpus](https://huggingface.co/datasets/bookcorpus).

The source code located in `cmd/smashwords-downloader`.

The `main.go` script takes the following arugments:
```
  -data_dir string
        directory that the book files will download to (default "./data")
```
