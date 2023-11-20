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

// extracted data will be transformed into this SeoData that we want to access
type SEOData struct {
	URL string
	Title string
	H1 string
	MetaDescription string
	StatusCode int
}

type Parser interface {
	getSEOData(resp *http.Response) (SEOData, error)
}

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

func isSitemap(urls []string) ([]string, []string) {
	sitemapFiles := []string{}
	pages := []string{}
	for _, page := range urls {
		foundSitemap := strings.Contains(page, "xml")
		if foundSitemap {
			fmt.Println("Found Sitemap", page)
			sitemapFiles = append(sitemapFiles, page)
		} else {
			pages = append(pages, page)
		}
	}
	return sitemapFiles, pages
}

// function to extract the URLs after a scrape
func extractSitemapURLs(startURL string) []string {
	worklist := make(chan []string)
	toCrawl := []string{}
	var n int
	n++
	go func() { worklist <- []string{startURL} }()

	for ; n > 0 ; n-- {

	list := <-worklist
	for _, link := range list{
		n++
		go func(link string) {
			response, err := makeRequest(link)
			if err != nil {
				log.Printf("Error retrieving URL:%s", link)
			}
			urls, _ := extractURLs(response)
			if err != nil {
				log.Printf("Error extracting document from response, URL:%s", link)
			}
			sitemapFiles, pages := isSitemap(urls)
			if sitemapFiles != nil {
				worklist <- sitemapFiles
			}
				toCrawl = append(toCrawl, pages...)
		}(link)
	}
}
	return toCrawl
}


func makeRequest(url string) (*http.Response, error) {
	client := http.Client{
		Timeout: 10*time.Second,
	}
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", randomUserAgent())
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func scrapeURLs(urls []string, parser Parser, concurrency int) []SEOData{
	tokens := make(chan struct{}, concurrency)
	var n int
	n++
	worklist := make(chan []string)
	results := []SEOData{}

	go func (){worklist <- urls}()
	for ; n > 0 ; n-- {
		list := <-worklist
		for _, url := range list {
			if url != "" {
				n++
				go func(url string, token chan struct{}) {
					log.Printf("Requesting URL:%s", url)
					res, err := scrapePage(url, tokens, parser)
					if err != nil {
						log.Printf("Encountered error, URL:%s", url)
					} else {
						results = append(results, res)
					}
					worklist <- []string{}
				}(url, tokens)
			}
		}
	}
	return results
}

func extractURLs(response *http.Response)([]string, error) {
	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return nil, err
	}
	results := []string{}
	sel := doc.Find("loc")
	for i := range sel.Nodes{
		loc := sel.Eq(i)
		result := loc.Text()
		results = append(results, result)
	}
	return results, nil
}

func scrapePage(url string, token chan struct{}, parser Parser) (SEOData, error) {
	res, err := crawlPage(url, token)
	if err != nil {
		return SEOData{}, err
	}
	data, err := parser.getSEOData(res)
	if err != nil {
		return SEOData{}, err
	}
	return data, nil
}

func crawlPage(url string, tokens chan struct{})(*http.Response, error) {
	tokens <- struct{}{}
	resp, err := makeRequest(url)
	if err != nil {
		return nil, err
	}
	return resp, err
}

func (d DefaultParser) getSEOData(resp *http.Response)(SEOData, error) {
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return SEOData{}, err
	}
	result := SEOData{}
	result.URL = resp.Request.URL.String()
	result.StatusCode = resp.StatusCode
	result.Title = doc.Find("title").First().Text()
	result.H1 = doc.Find("h1").First().Text()
	result.MetaDescription,_ = doc.Find("meta[name^=description]").Attr("content")
	return result, nil
}

// function to scrape the sitemap and call the functions to extract the URLs we want to crawl
func ScrapeSitemap(url string, parser Parser, concurrency int) []SEOData{
	results := extractSitemapURLs(url)
	res := scrapeURLs(results, parser, concurrency)
	return res
}

func main() {
	p := DefaultParser{}
	results := ScrapeSitemap("https://www.freecodecamp.org/news/sitemap.xml", p, 10)
	for _, res := range results {
		fmt.Println(res)
	}
}