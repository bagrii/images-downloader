// Images downloader from a webapge.
package main

import (
	"flag"
	"log"

	"onethinglab.com/imagedown/downloader"
)

func main() {
	var (
		baseURL   = flag.String("--url", "https://onethinglab.com", "Specify URL to download images from.")
		outputDir = flag.String("--dir", "/tmp/", "Specify directory where images will be stored.")
		feedback  = make(chan downloader.DownloadEntry)
	)

	log.Println("Downloading images from:", *baseURL, "to:", *outputDir)

	go downloader.DownloadImages(*baseURL, *outputDir, feedback)

	for entry := range feedback {
		if entry.Error != nil {
			log.Println("Error occurred while dowloading image: ", entry.Error)
		} else {
			log.Printf("Downloading %s\n", entry.Filename)
		}
	}

	log.Printf("Done.")
}
