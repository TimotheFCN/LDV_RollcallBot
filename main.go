package main

import (
	"crypto/tls"
	"encoding/base64"
	"github.com/PuerkitoBio/goquery"
	"github.com/apognu/gocal"
	"github.com/joho/godotenv"
	"github.com/madflojo/tasks"
	"github.com/rs/xid"
	"io"
	log2 "log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

var scheduler *tasks.Scheduler
var login string
var password string
var notifID string
var baseURL = "https://www.leonard-de-vinci.net"
var client http.Client
var lessonList []Lesson

var debug = false

type Lesson struct {
	Description string
	Zoomlink    string
	rollCallURL string
	StartTime   time.Time
	EndTime     time.Time
	isOpen      bool
}

func log(message string) {
	println(time.Now().Format("02-01 15:04:05") + " [INFO] " + message)
}
func logDebug(message string) {
	if debug {
		println(time.Now().Format("02-01 15:04:05") + " [DEBUG] " + message)
	}
}

func isAlreadyListed(lesson Lesson) bool {
	for _, l := range lessonList {
		if l.StartTime == lesson.StartTime {
			return true
		}
	}
	return false
}
func getDecodedFireBaseKey() ([]byte, error) {

	fireBaseAuthKey := os.Getenv("FIREBASE_AUTH_KEY")

	decodedKey, err := base64.StdEncoding.DecodeString(fireBaseAuthKey)
	if err != nil {
		return nil, err
	}

	return decodedKey, nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log(".env file not found, trying to use env variables")
	}

	scheduler = tasks.New()
	defer scheduler.Stop()

	login = os.Getenv("LOGIN")
	password = os.Getenv("PASSWORD")
	notifID = os.Getenv("NOTIFID")
	debug = os.Getenv("DEBUG") == "true"

	sendMessageNotification("Prêt à valider, bisouuu je manvol")

	jar, err := cookiejar.New(nil)

	client = http.Client{
		Jar: jar,
	}

	authCookies()
	getCalendar()

	/*	//run getcalendar every 12 hours
		ticker := time.NewTicker(time.Hour * 12)
		quit := make(chan struct{})
		go func() {
			for {
				select {
				case <-ticker.C:
					getCalendar()
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()
		<-quit //wait for the goroutine to finish*/

	//run getcalendar at 05:00 and 12:00 every day
	_, err = scheduler.Add(&tasks.Task{
		StartAfter: time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()+1, 5, 0, 0, 0, time.Local),
		Interval:   time.Duration(24) * time.Hour,
		TaskFunc: func() error {
			getCalendar()
			return nil
		},
	})
	logDebug("Added task to get calendar at 05:00")
	logError(err)
	_, err = scheduler.Add(&tasks.Task{
		StartAfter: time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()+1, 12, 00, 0, 0, time.Local),
		Interval:   time.Duration(24) * time.Hour,
		TaskFunc: func() error {
			getCalendar()
			return nil
		},
	})
	logDebug("Added task to get calendar at 12:00")
	logError(err)

	//run getcalendar tomorrow at 6:00 (required for the first run because the scheduler runs the task at the next interval)
	_, err = scheduler.Add(&tasks.Task{
		StartAfter: time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()+1, 6, 00, 00, 0, time.Local),
		RunOnce:    true,
		Interval:   time.Duration(1) * time.Second,
		TaskFunc: func() error {
			getCalendar()
			return nil
		},
	})
	logError(err)

	//loop forever
	select {}
}

func logError(err error) {
	if err != nil {
		print("[ERROR] ")
		log2.Fatal(err)
	}
}

func authCookies() {
	log("Getting auth cookies")
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
		if samlResponse == "" {
			log("Authentification failed")
			sendMessageNotification("Erreur: Informations de connexion incorrectes.")
			os.Exit(1)
		}
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
	logError(err)
	log("Authenticated")
}

func getCalendar() {
	reAuth()
	calLink := func() string {
		resp, err := client.Get(baseURL)
		logError(err)
		defer resp.Body.Close()
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		logError(err)
		link := doc.Find("#main > div:nth-child(3) > div:nth-child(3) > div.app-root > div.body > a").AttrOr("href", "error")
		if link == "error" {
			log("Error while getting calendar link")
			return ""
		}
		return link
	}
	calendar := func() []gocal.Event {
		ical, err := client.Get(calLink())
		log("Calendar: " + calLink())
		logError(err)
		defer ical.Body.Close()
		//Parse the ical
		//start now and end at 00:00:00
		start, end := time.Now(), time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 23, 59, 59, 0, time.Local)
		c := gocal.NewParser(ical.Body)
		c.Start, c.End = &start, &end
		c.Parse()
		return c.Events
	}

	for _, event := range calendar() {
		lesson := Lesson{
			Description: event.Summary + " à " + event.Start.Format("15:04"),
			Zoomlink:    strings.TrimSpace(event.Description),
			StartTime:   event.Start.Local(),
			EndTime:     event.End.Local(),
		}
		if !isAlreadyListed(lesson) {
			lessonList = append(lessonList, lesson)
			getrollCallURl(lesson) //If available, otherwise it's null
			//checkOpen(lesson)
			go schedule(lesson)
		}
	}
	log("Calendar updated")
}

func schedule(lesson Lesson) {
	//Schedule the getrollCall when the lesson starts
	_, err := scheduler.Add(&tasks.Task{
		StartAfter: lesson.StartTime,
		RunOnce:    true,
		TaskFunc: func() error {
			getrollCallURl(lesson)
			return nil
		},
	})
	lesson.rollCallURL = getrollCallURl(lesson)
	//Schedule the checkopen until the lesson ends or the rollCall is opened
	id := xid.New()
	err = scheduler.AddWithID(id.String(), &tasks.Task{
		Interval:   time.Duration(time.Minute * 2),
		StartAfter: lesson.StartTime,
		TaskFunc: func() error {
			if checkOpen(lesson) {
				scheduler.Del(id.String())
				log("Task " + lesson.Description + " removed, rollcall opened")
				return nil
			}
			//if the lesson is finished, remove the task
			if time.Now().After(lesson.EndTime) {
				scheduler.Del(id.String())
				log("Task " + lesson.Description + " removed, lesson finished")
				return nil
			}
			return nil
		},
	})
	log("Task " + lesson.Description + " scheduled")
	logError(err)
}

func getrollCallURl(lesson Lesson) string {
	//Get the rollCall url from the zoom link
	var row []string
	var rows [][]string

	resp, err := client.Get(baseURL + "/student/presences/")
	logError(err)
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	logError(err)
	doc.Find("table").Each(func(index int, tablehtml *goquery.Selection) {
		tablehtml.Find("tr").Each(func(indextr int, rowhtml *goquery.Selection) {
			rowhtml.Find("td").Each(func(indexth int, tablecell *goquery.Selection) {
				links, _ := tablecell.Find("a").Attr("href")
				row = append(row, links)
			})
			rows = append(rows, row)
			row = nil
		})
	})
	for _, row := range rows {
		if len(row) > 5 {
			link := strings.TrimSpace(row[4]) //remove spaces
			if link == lesson.Zoomlink {
				logDebug("Found rollcall url for " + lesson.Description)
				return strings.TrimSpace(row[3])
			}
		}
	}
	logDebug("No rollcall URL found for " + lesson.Description)
	return ""
}

func checkOpen(lesson Lesson) bool {
	log("Checking if rollCall is open for " + lesson.Description)
	logDebug("URL: " + lesson.rollCallURL)
	//Check if the rollCall is opened
	resp, err := client.Get(baseURL + lesson.rollCallURL)
	logError(err)
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	logError(err)
	//check if the auth is still valid
	el := doc.Find("#formWrapper > div.forget-password")
	if el.Nodes != nil || len(el.Nodes) > 0 {
		log("Token expired, reauthenticating")
		authCookies()
	}
	//Check if the rollcall is already validated
	checkValidated := doc.Find("#body_presence > div").Text()
	if strings.Contains(checkValidated, "Vous avez été noté présent") {
		return true
	}
	checkopen := doc.Find("#set-presence").Text()
	if checkopen != "" {
		log("Roll call is open for " + lesson.Description)
		lesson.isOpen = true
		sendNotification(lesson)
	}
	return checkopen != ""
}

/*func sendNotification(lesson Lesson) {
	_, err := fcmClient.SendMulticast(context.Background(), &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: "Appel ouvert",
			Body:  "L'appel est ouvert pour le cours " + lesson.Description,
		},
		Tokens: deviceTokens,
	})
	logError(err)
}*/

func sendNotification(lesson Lesson) {
	targetURL, err := url.Parse("https://alertzy.app/send")
	logError(err)

	formData := url.Values{}
	formData.Set("accountKey", notifID)
	formData.Set("title", "Appel ouvert !")
	formData.Set("message", lesson.Description)

	req, err := http.NewRequest("POST", targetURL.String(), strings.NewReader(formData.Encode()))
	logError(err)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Do(req)
	logError(err)

	defer resp.Body.Close()
}

func sendMessageNotification(message string) {
	targetURL, err := url.Parse("https://alertzy.app/send")
	logError(err)

	formData := url.Values{}
	formData.Set("accountKey", notifID)
	formData.Set("title", "RollCallBot")
	formData.Set("message", message)

	req, err := http.NewRequest("POST", targetURL.String(), strings.NewReader(formData.Encode()))
	logError(err)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Do(req)
	logError(err)

	defer resp.Body.Close()
}

// Check if the token is still valid
// If not, reauthenticate
func reAuth() {
	resp, err := client.Get(baseURL)
	logError(err)
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	logError(err)
	el := doc.Find("#formWrapper > div.forget-password")
	if el.Nodes != nil || len(el.Nodes) > 0 {
		log("Token expired, reauthenticating")
		authCookies()
	}
}

func validate(lesson Lesson) {

}
