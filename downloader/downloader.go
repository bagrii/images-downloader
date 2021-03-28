// Copyright (c) 2021 Bagrii Petro.
//
// downloader.go implements:
//  - Extracting images from <a>, <img>, <svg>, <iframe>, <object>, <link>, <embed> elements.
//  - Downloading images concurrently.
//  - Parsing Data UL into internal representation: https://tools.ietf.org/html/rfc2397



package downloader

import (
	"crypto/tls"
	"context"
	"runtime"
	"errors"
	"fmt"
	"log"
	"os"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/sync/semaphore"
)

type elementType int
type dataType int
type elementConent struct {
	contentType elementType
	dataType    dataType
	dataExt     string
	data        string
}
type nodeParseCallback func(node *html.Node) (*elementConent, error)

const (
	aElement elementType = iota
	imgElement
	svgElement
	pictureElement
	iframeElement
	objectElemet
	linkElement
	embedElement
)

const (
	dataURL dataType = iota
	dataInline
)

var domHandlers = map[string]nodeParseCallback{
	"a": parseA, "img": parseIMG,
	"svg": parseSVG, "iframe": parseIframe,
	"object": parseObject,
	"link":   parseLink,
	"embed":  parseEmbed,
}


func (dt dataType) String() string {
	switch dt {
	case dataURL:
		return "dataURL"
	case dataInline:
		return "dataInline"
	}

	return fmt.Sprintf("Unknowne dataType: %d", dt)
}

func (element elementType) String() string {

	switch element {
	case aElement:
		return "<a>"
	case imgElement:
		return "<img>"
	case svgElement:
		return "<svg>"
	case pictureElement:
		return "<picture>"
	case iframeElement:
		return "<iframe>"
	case objectElemet:
		return "<object>"
	case linkElement:
		return "<link>"
	case embedElement:
		return "<embed>"
	}

	return "unknown element"
}

func getAttr(node *html.Node, name string) (string, bool) {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val, true
		}
	}

	return "", false
}

func resolveURL(baseURL, inURL string) (string, error) {
	parsedURL, err := url.Parse(inURL)

	if err != nil {
		return "", err
	}
	// already in correct format
	if parsedURL.IsAbs() {
		return inURL, nil
	}

	if len(parsedURL.Scheme) == 0 {
		// handing URL that start with double slash: "//example.com"
		if len(parsedURL.Host) > 0 {
			// use https by default
			parsedURL.Scheme = "https"
		} else {
			// handling relative local path
			base, err := url.Parse(baseURL)
			if err != nil {
				return "", err
			}
			parsedURL = base.ResolveReference(parsedURL)
		}
	}

	return parsedURL.String(), nil
}

func tryParseImageDataURL(url string, content *elementConent) (bool, error) {
	var isImage bool
	data, err := ParseDataURL(url)

	if err != nil {
		return false, err
	}
	if data.Type == "image" {
		mimeType := data.Type + "/" + data.Subtype
		exts, found := MimeTypeToExt[mimeType]
		if isImage = found; isImage {
			if !data.IsBase64 {
				log.Printf("Mime type %s in an image, but without base64 propertie?", mimeType)
			}
			content.data = data.Data
			// get the first one from extension list
			content.dataExt = exts[0]
			content.dataType = dataInline
		} else {
			log.Printf("Mime type for image not found for %s", mimeType)
		}
	}

	return isImage, nil
}

func parseEmbeddableObject(node *html.Node, typeAttr string,
	dataAttr string, contentType elementType) (*elementConent, error) {
	data, exist := getAttr(node, dataAttr)
	if !exist || len(data) == 0 {
		return nil, fmt.Errorf("'%s' does not exists in %s element",
			dataAttr, contentType)
	}
	// check if mime type is an image.
	var mimeExt string
	type_, exist := getAttr(node, typeAttr)
	if exist {
		appType := strings.Split(type_, "/")
		if appType[0] != "image" {
			return nil, nil
		}
		if exts, found := MimeTypeToExt[type_]; found {
			mimeExt = exts[0]
		} else {
			log.Printf("Mime type is image, but can't match extension for %s\n", type_)
		}
	}

	if IsDataURL(data) {
		content := elementConent{}
		if isImage, _ := tryParseImageDataURL(data, &content); isImage {
			content.contentType = contentType
			return &content, nil
		}
	} else if ext := path.Ext(data); len(ext) == 0 {
		if exist {
			return &elementConent{contentType, dataURL, mimeExt, data}, nil
		}
	} else if ext = ext[1:]; IsImageExtension(ext) {
		return &elementConent{contentType, dataURL, ext, data}, nil
	}

	return nil, nil

}

func parseA(node *html.Node) (*elementConent, error) {
	var result *elementConent
	var err error

	if href, exist := getAttr(node, "href"); exist {
		if IsDataURL(href) {
			var isImage bool
			content := elementConent{}
			isImage, err = tryParseImageDataURL(href, &content)
			if isImage {
				content.contentType = aElement
				result = &content
			}
		} else if ext := filepath.Ext(href); len(ext) > 0 {
			// remove leading dot
			ext = ext[1:]
			if IsImageExtension(ext) {
				result = &elementConent{aElement, dataURL, ext, href}
			}
		}
	} else {
		err = errors.New("'href' attribute not found in <a> element")
	}

	return result, err
}

func parseIMG(node *html.Node) (*elementConent, error) {
	src, exist := getAttr(node, "src")
	if !exist || len(src) == 0 {
		return nil, errors.New("'src' attribute not found or empty in <img> element")
	}

	if IsDataURL(src) {
		content := elementConent{}
		if isImage, _ := tryParseImageDataURL(src, &content); isImage {
			content.contentType = imgElement
			return &content, nil
		} else {
			return nil, fmt.Errorf("unrecognized image in the Data URL %s", src)
		}
	} else {
		ext := filepath.Ext(src)
		if len(ext) > 0 {
			// remove leading dot
			ext = ext[1:]
			if !IsImageExtension(ext) {
				fmt.Printf("extension %s is not recognized as image extension.", ext)
				ext = ""
			}
		}
		return &elementConent{imgElement, dataURL, ext, src}, nil
	}
}

func parseSVG(node *html.Node) (*elementConent, error) {
	var text strings.Builder
	if err := html.Render(&text, node); err != nil {
		return nil, err
	}
	return &elementConent{svgElement, dataInline,
		"svg", text.String()}, nil
}

func parseIframe(node *html.Node) (*elementConent, error) {
	src, exist := getAttr(node, "src")
	if !exist || len(src) == 0 {
		return nil, errors.New("'src' attribute not found or empty in <iframe> element")
	}

	if IsDataURL(src) {
		content := elementConent{}
		if isImage, _ := tryParseImageDataURL(src, &content); isImage {
			content.contentType = iframeElement
			return &content, nil
		}
	} else if ext := path.Ext(src); len(ext) > 0 {
		ext = ext[1:]
		if IsImageExtension(ext) {
			return &elementConent{iframeElement, dataURL, ext, src}, nil
		}
	}

	return nil, nil
}

func parseObject(node *html.Node) (*elementConent, error) {
	return parseEmbeddableObject(node, "type", "data", objectElemet)
}

func parseEmbed(node *html.Node) (*elementConent, error) {
	return parseEmbeddableObject(node, "type", "src", embedElement)
}

func parseLink(node *html.Node) (*elementConent, error) {
	href, exist := getAttr(node, "href")

	if !exist || len(href) == 0 {
		return nil, errors.New("'href' does not exists in <link> element")
	}

	if IsDataURL(href) {
		content := elementConent{}
		if isImage, _ := tryParseImageDataURL(href, &content); isImage {
			content.contentType = linkElement
			return &content, nil
		}
	} else if ext := path.Ext(href); len(ext) > 0 {
		if ext = ext[1:]; IsImageExtension(ext) {
			return &elementConent{linkElement, dataURL, ext, href}, nil
		}
	}

	return nil, nil
}

func iterateDOM(root *html.Node, baseURL string,
	callbacks map[string]nodeParseCallback) []*elementConent {
	queue, elements := make([]*html.Node, 0), make([]*elementConent, 0)

	queue = append(queue, root)

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if callback, exist := callbacks[strings.ToLower(node.Data)]; exist {
			if content, err := callback(node); err == nil && content != nil {
				if content.dataType == dataURL {
					if fullURL, err := resolveURL(baseURL, content.data); err == nil {
						content.data = fullURL
					}
				}
				elements = append(elements, content)
			}

		}
		for n := node.FirstChild; n != nil; n = n.NextSibling {
			if n.Type == html.ElementNode {
				queue = append(queue, n)
			}
		}
	}
	return elements
}

func getHTTPClient(tlsvetify bool) *http.Client {
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	// `tlsvetify` indocates whether to ignore expired or not valid certificate.
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: tlsvetify}
	client := &http.Client{Transport: customTransport}

	return client
}

func downloadImage(content *elementConent, dir string) (string, error) {
	client := getHTTPClient(true)

	if content.dataType == dataInline {
		file, err := os.CreateTemp(dir, "*."+content.dataExt)
		if err != nil {
			return "", err
		}
		defer file.Close()

		if _, err := file.WriteString(content.data); err != nil {
			return "", err
		}
		return path.Join(dir, file.Name()), nil
	} else if content.dataType == dataURL {
		resp, err := client.Get(content.data)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("received response code, %d", resp.StatusCode)
		}

		filename := path.Base(content.data)
		if ext := path.Ext(filename); len(ext) == 0 && len(content.dataExt) > 0 {
			filename += "." + content.dataExt
		}
		filename = path.Join(dir, filename)
		file, err := os.Create(filename)
		if err != nil {
			return "", err
		}
		defer file.Close()

		if _, err = io.Copy(file, resp.Body); err != nil {
			return "", err
		}

		return filename, nil
	}
	return "", fmt.Errorf("unknown data type: %s", content.dataType)
}

// DownloadEntry represent downloaded file.
type DownloadEntry struct {
	Filename string
	Error error
}

func parseHTML(baseURL string) (*html.Node, error) {
	client := getHTTPClient(true)
	resp, err := client.Get(baseURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received response code, %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-type")
	if mediatype, _, err := mime.ParseMediaType(contentType); err != nil {
		return nil, err
	} else if mediatype != "text/html" {
		return nil, fmt.Errorf("incorrect media type: %s", mediatype)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// DownloadImages download all images from URL and save to directory.
func DownloadImages(baseURL string, dir string, feedback chan DownloadEntry) {
	var (
		maxWorkers = runtime.GOMAXPROCS(0)
		sem        = semaphore.NewWeighted(int64(maxWorkers))
	)

	defer close(feedback)

	root, err := parseHTML(baseURL)
	
	if err != nil {
		feedback <- DownloadEntry{Error: err}
		return
	}

	getImage := func(content *elementConent) {
		defer sem.Release(1)

		filename, err := downloadImage(content, dir)
		feedback <- DownloadEntry{filename, err}
	}

	ctx := context.TODO()
	for _, content := range iterateDOM(root, baseURL, domHandlers) {
		if err := sem.Acquire(ctx, 1); err != nil {
			feedback <- DownloadEntry{Error: err}
			return
		}

		go getImage(content)
	}

	if err := sem.Acquire(ctx, int64(maxWorkers)); err != nil {
		feedback <- DownloadEntry{Error: err} 
	}
}
