package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/apognu/gocal"
	"github.com/joho/godotenv"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"
)

var login string
var password string
var baseURL = "https://www.leonard-de-vinci.net"
var client http.Client

func main() {
	//taskScheduler := chrono.NewDefaultTaskScheduler()
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	login = os.Getenv("LOGIN")
	password = os.Getenv("PASSWORD")

	jar, err := cookiejar.New(nil)

	client = http.Client{
		Jar: jar,
	}

	authCookies()
	getCalendar()
}

func logError(err error) {
	if err != nil {
		print("[ERROR] ")
		log.Fatal(err)
	}
}

func authCookies() {
	samlLink := func() string {
		//Get SAML Link from ajax
		resp, err := client.PostForm(baseURL+"/ajax.inc.php", url.Values{"act": {"ident_analyse"}, "login": {login}})
		logError(err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		logError(err)
		//Trim the link from the response
		samlLink := string(body[25 : len(body)-3])
		return samlLink
	}
	adfsURL := func() string {
		//Get ADFS URL from SAML Link
		resp, err := client.Get(baseURL + samlLink())
		logError(err)
		defer resp.Body.Close()
		//Get the URL
		adfsURL := resp.Request.URL.String()
		return adfsURL
	}
	samlResponse := func() string {
		//Get the cookies from ADFS by posting the login and password
		resp, err := client.PostForm(adfsURL(), url.Values{"UserName": {login}, "Password": {password}, "AuthMethod": {"FormsAuthentication"}})
		logError(err)
		defer resp.Body.Close()
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		logError(err)
		//Get the SAMLResponse from the form
		samlResponse, _ := doc.Find("input[name=SAMLResponse]").Attr("value")
		return samlResponse
	}
	//Post the SAMLResponse to simpleSAML
	resp, err := client.PostForm(baseURL+"/simplesaml/module.php/saml/sp/saml2-acs.php/alv-sp", url.Values{"SAMLResponse": {samlResponse()}, "RelayState": {samlLink()}})
	logError(err)
	defer resp.Body.Close()
	//Go to the saml link to get the cookies
	resp, err = client.Get(baseURL + samlLink())
	logError(err)
	defer resp.Body.Close()
	//print the body to see if it worked
	_, err = io.ReadAll(resp.Body)
	logError(err)
	println("[INFO] Authenticated")
}

func getCalendar() {
	calLink := func() string {
		resp, err := client.Get(baseURL)
		logError(err)
		defer resp.Body.Close()
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		logError(err)
		return doc.Find("#main > div:nth-child(3) > div > div > div > header > div > a").AttrOr("href", "error")
	}
	calendar := func() []gocal.Event {
		ical, err := client.Get(calLink())
		logError(err)
		defer ical.Body.Close()
		//Parse the ical
		start, end := time.Now(), time.Now().Add(7*24*time.Hour)
		c := gocal.NewParser(ical.Body)
		c.Start, c.End = &start, &end
		c.Parse()
		return c.Events
	}
	for _, event := range calendar() {
		println(event.Summary + " de " + event.Start.Format("15:04") + " à " + event.End.Format("15:04"))
	}
}
