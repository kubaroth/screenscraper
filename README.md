# Overview
A tool to automate screen capturing process, in particular long web pages or image sequences. The output is saved into a pdf file.
Currently only support platform is Linux with X windowing system.

![](images/demo.gif)

## Use:
```
screenscraper
```
Once a captured area is selected the focused window is automatically changed to the target window and capturing begins. Once two consecutive pages with the same content are detected the capturing process stops and a pdf file is generated.

### Parameters

- -w Name of the window to capture. The default window name is 'Chrom' which will Chrome and Chromium on some platforms. (default "Chrom")
- -p Total numer of pages to capture. The default -1, does not interupt capturing. (default -1)
 
## Build:
```
go build screenscraper.go
```
 
## Preview:
```
evince /tmp/generated_pdf_file.pdf
```
