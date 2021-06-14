# Overview
A tool to automate process of screen capturing of long web pages or image sequence. The output is saved into a pdf file.
Currently only support platform is Linux with X windowing system.

## Use:
```
screenscraper
```
Once a capture region is selected the window focus automatically changes to the target window (currently Chrome) and capturing begins. Once two consecutive pages with the same content are encountered the capturing process stops and a pdf file is generated.
 
## Build:
```
go build screenscraper.go
```
 
## Preview:
```
evince /tmp/generated_pdf_file.pdf
```
