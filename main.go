package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// extracted data will be transformed into this SEOData that we want to access
type SEOData struct {
	URL string
	Title string
	H1 string
	MetaDescription string
	StatusCode int
}

// defines parsing interface
type Parser interface {
	GetSEOData(resp *http.Response) (SEOData, error)
}

// empty struct for implementing the default parser
type DefaultParser struct {

}

// create userAgents to avoid overloading the site with requests from the same user, or to avoid being blacklisted
var userAgents = []string {
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.0.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36",

}

// function to pick a random user agent for making requests
func randomUserAgent() string {
	rand.New(rand.NewSource(time.Now().Unix()))
	randNum := rand.Int() % len(userAgents)
	return userAgents[randNum]
}

// takes a slice of URLs and categorizes them into two slices : sitemapFiles for confirmed sitemaps and pages for other URLs
func isSitemap(urls []string) ([]string, []string) {
	// build a slice of confirmed sitemaps
	sitemapFiles := []string{}
	// build a slice of the pages that aren't sitemaps
	pages := []string{}
	// iterate over each URL in the input slice
	for _, page := range urls {
		// check for matching substring 'xml' and set in variable as confirmed sitemap
		foundSitemap := strings.Contains(page, "xml")
		// if the URL is a sitemap, add the page to the sitemapFiles slice
		if foundSitemap {
			fmt.Println("Found Sitemap", page)
			sitemapFiles = append(sitemapFiles, page)
			// otherwise add it to the pages slice
		} else {
			pages = append(pages, page)
		}
	}
	return sitemapFiles, pages
}

// function to extract the URLs after a scrape
func extractSitemapURLs(startURL string) []string {
	// create a worklist channel
	worklist := make(chan []string)
	// slice to contain all the pages to crawl over
	toCrawl := []string{}
	// set a variable n for looping over the worklist and initiate it
	var n int
	n++
	// send the base sitemap URL to the worklist channel
	go func() { worklist <- []string{startURL} }()
	// start the loop
	for ; n > 0; n-- {
		// set the worklist channel in a variable and range over it
		list := <-worklist
		// loop over the list of links
		for _, link := range list{
			n++
			// run goroutine to make requests concurrently and check each link
			go func(link string) {
				// set the response of our request in a variable
				response, err := makeRequest(link)
				if err != nil {
					log.Printf("Error retrieving URL: %s", link)
				}
				// extract the URLs from the response from our request and set it in a variable urls
				urls, _ := extractURLs(response)
				if err != nil {
					log.Printf("Error extracting document from response, URL: %s", link)
				}
				// recuperate the returned values of sitemapFiles and pages from the isSitemap function with our urls checked
				sitemapFiles, pages := isSitemap(urls)
				// if there are sitemap files, add them to the worklist channel
				if sitemapFiles != nil {
					worklist <- sitemapFiles
				}
				// append the pages to the toCrawl slice which contains our pages to crawl
				toCrawl = append(toCrawl, pages...)
			}(link)
		}
}
	return toCrawl
}

// function to make a request to a url and receive a response
func makeRequest(url string) (*http.Response, error) {
	// set the http client in a variable
	client := http.Client{
		// set a timeout for the request
		Timeout: 10*time.Second,
	}
	// set the request in a variable req and manage the error
	req, err := http.NewRequest("GET", url, nil)
	// set the request header for the user-agent so a random user agent is used every time
	req.Header.Set("User-Agent", randomUserAgent())
	if err != nil {
		return nil, err
	}
	// recuperate the response in a variable res
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// function to scrape the confirmed sitemap URLs and recuperate the SEOData we want
func scrapeURLs(urls []string, parser Parser, concurrency int) []SEOData{
	// create a tokens channel and make use concurrency
	tokens := make(chan struct{}, concurrency)
	// set a variable for the endless loop
	var n int
	// initiate the variable
	n++
	// set a worklist channel to coordinate tasks between goroutines
	worklist := make(chan []string)
	// create a slice results for all of the SEOData we recuperate from all of the sites
	results := []SEOData{}
	// run a goroutine to send all of the urls to the worklist and then loop over it
	go func() {worklist <- urls}()
	// start the counter
	for ; n > 0 ; n-- {
		// set the contents of the worklist channel in a list variable
		list := <-worklist
		// range over the list
		for _, url := range list {
			if url != "" {
				n++
				// goroutine which scrapes each page for our SEOData
				go func(url string, token chan struct{}) {
					log.Printf("Requesting URL: %s", url)
					// set the results of our scrapePage function in a variable
					res, err := scrapePage(url, tokens, parser)
					if err != nil {
						log.Printf("Encountered error, URL: %s", url)
					} else {
						// append the res (SEOData) to our results slice
						results = append(results, res)
					}
					// signal that the goroutine has completed its work by adding an empty slice to the worklist channel
					worklist <- []string{}
				}(url, tokens)
			}
		}
	}
	// return all of the collected SEOData in the slice results
	return results
}

// extracts URLs from the html content of an http response
func extractURLs(response *http.Response)([]string, error) {
	// parse the html content of the response into a doc variable
	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return nil, err
	}
	// build a slice of strings containing the extracted URLs
	results := []string{}
	// find all elements in the document with the tag name 'loc'
	sel := doc.Find("loc")
	// iterate over what we found
	for i := range sel.Nodes{
		// select the current element and put it in a variable loc
		loc := sel.Eq(i)
		// set the text content of the element (URL) in a variable and append it to the results slice
		result := loc.Text()
		results = append(results, result)
	}
	return results, nil
}

// crawls a page and extracts SEOData with the parser
func scrapePage(url string, token chan struct{}, parser Parser) (SEOData, error) {
	// recuperate the url and its token in a variable res then get the SEOData associated with it
	res, err := crawlPage(url, token)
	if err != nil {
		return SEOData{}, err
	}
	// parse the data in res and put it in a variable data
	data, err := parser.GetSEOData(res)
	if err != nil {
		return SEOData{}, err
	}
	return data, nil
}

// function that controls making requests to URLs by acquiring a token before the request and releasing it after
func crawlPage(url string, tokens chan struct{})(*http.Response, error) {
	// claim a token by sending an empty struct to the channel
	tokens <- struct{}{}
	resp, err := makeRequest(url)
	// release the token back to the channel
	<-tokens
	if err != nil {
		return nil, err
	}
	return resp, err
}

// DefaultParser type, takes an HTTP response, extracts relevant information and then populates an SEOData struct with the results
func (d DefaultParser) GetSEOData(resp *http.Response)(SEOData, error) {
	// parse the response body into a variable doc
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return SEOData{}, err
	}
	// set the SEOData into a variable result
	result := SEOData{}
	// set all of the fields of the struct SEOData with data from the response
	result.URL = resp.Request.URL.String()
	result.StatusCode = resp.StatusCode
	result.Title = doc.Find("title").First().Text()
	result.H1 = doc.Find("h1").First().Text()
	result.MetaDescription,_ = doc.Find("meta[name^=description]").Attr("content")
	return result, nil
}

// function to scrape the sitemap and call the functions to extract the URLs we want to crawl and recuperate SEOData from them
func ScrapeSitemap(url string, parser Parser, concurrency int) []SEOData{
	results := extractSitemapURLs(url)
	res := scrapeURLs(results, parser, concurrency)
	return res
}

func main() {
	p := DefaultParser{}
	// set the site to scrape, the parser, and the concurrency limit which is used in the various functions in the program to limit the number of goroutines that can be used concurrently through the use of tokens
	results := ScrapeSitemap("https://www.freecodecamp.org/news/sitemap.xml", p, 10)
	for _, res := range results {
		fmt.Println(res)
	}
}