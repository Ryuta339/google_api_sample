package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

const (
	credentials string = "client_id.json"
	dist        string = "dist"

	tokFile string = "token.json"

	scopes = drive.DriveScope
)

var folderId string
var errorFileNames []string
var printFlag *bool

// contains?
// return the first index if src contains elem
//        -1 otherwise
func contains(src []string, elem string) int {
	for i, v := range src {
		if v == elem {
			return i
		}
	}
	return -1
}

// inputFolderId
//    requires to input folder id from stdin
func inputFolderId() {
	sc := bufio.NewScanner(os.Stdin)
	fmt.Println("Input google folder id.")

	if sc.Scan() {
		folderId = sc.Text()
	}
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// Reissue access tokens
func ReissueTokens(config *oauth2.Config, token *oauth2.Token) error {
	requestUrl := "https://www.googleapis.com/oauth2/v4/token"
	// fmt.Println(config.Endpoint.TokenURL)
	req, err := http.NewRequest("POST", requestUrl, nil)
	if err != nil {
		fmt.Printf("An error occurred: %v\n", err)
		return err
	}
	//reft := token.RefreshToken[:1] + token.RefreshToken[2:]
	req.Header.Set("refresh_token", token.RefreshToken)
	//req.Header.Set("refresh_token", reft)
	req.Header.Set("access_type", "offline")
	req.Header.Set("client_id", config.ClientID)
	req.Header.Set("client_secret", config.ClientSecret)
	req.Header.Set("redirect_uri", config.RedirectURL)
	req.Header.Set("grant_type", "refresh_token")
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	dump, _ := httputil.DumpRequestOut(req, true)
	fmt.Printf("%s\n\n", dump)

	client := new(http.Client)
	resp, err := client.Do(req)

	fmt.Println(resp.StatusCode)
	// dumpResp, _ := httputil.DumpResponse(resp, true)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	SaveFile(data, "response.html")

	// fmt.Printf("%s\n", dumpResp)

	return nil
}

// PrintFile fetches and displays the given file.
func PrintFile(d *drive.Service, fileId string) error {
	f, err := d.Files.Get(fileId).
		Fields("parents, id, name, mimeType").Do()
	if err != nil {
		fmt.Printf("An error occurred: %v\n", err)
		return err
	}
	fmt.Printf("Name: %v ", f.Name)
	fmt.Printf("MIME type: %v ", f.MimeType)
	fmt.Printf("Parent: %v ", f.Parents)
	fmt.Println("")
	return nil
}

// Printout print data on a printer.
func Printout(data []byte) {
	pr, pw := io.Pipe()
	c := exec.Command("lpr")
	c.Stdin = pr

	go func() {
		buf := bytes.NewBuffer(data)
		buf.WriteTo(pw)
		pw.Close()
	}()

	c.Start()
	c.Wait()
}

// Save data
func SaveFile(data []byte, fileName string) {
	ioutil.WriteFile(fileName, data, 0644)
}

// DownloadFile fetches and downloads the given file as mimetype pdf
func DownloadFile(d *drive.Service, fileId string, fileName string) error {
	response, err := d.Files.Export(fileId, "application/pdf").Fields("size(A4), fitw(true)").Download()
	// response, err := d.Files.Get (fileId).Download ()
	if err != nil {
		fmt.Printf("An error occurred: %v\n", err)
		return err
	}
	defer response.Body.Close()
	data, err := ioutil.ReadAll(response.Body)

	// Print on a printer
	// Printout(data)

	// Save as pdf
	SaveFile(data, fileName)

	return nil
}

// FromSpreadsheetToPdf fetches and downloads the given spreadsheet
//     with A4 paper size
func FromSpreadsheetToPdf(file *drive.File, config *oauth2.Config) error {
	if file.MimeType != "application/vnd.google-apps.spreadsheet" {
		fmt.Printf("Mimetype is not spreadsheet: %s\n", file.MimeType)
		return nil
	}
	token, err := tokenFromFile("token2.json")
	if err != nil {
		token = getTokenFromWeb(config)
		saveToken("token2.json", token)
	}

	if !token.Valid() {
		// if expired
		ReissueTokens(config, token)
		return nil
	}

	url := "https://docs.google.com/spreadsheets/d/" + file.Id + "/export?" +
		"format=pdf" +
		"&size=A4" +
		"&fitw=true" +
		"&gid=0" + // 0?
		"&access_token=" + token.AccessToken

	req, err := http.NewRequest("Get", url, nil)
	if err != nil {
		fmt.Printf("An error occurred: %v\n", err)
		return err
	}
	client := new(http.Client)
	response, _ := client.Do(req)

	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("%d\t", response.StatusCode)
	} else {
		fmt.Printf("\x1b[31m%d\t", response.StatusCode)
		errorFileNames = append(errorFileNames, file.Name)
		return errors.New("Response status is not 2xx.")
	}
	data, err := ioutil.ReadAll(response.Body)

	// Save as pdf
	fileName := dist + "/" + file.Name + ".pdf"
	SaveFile(data, fileName)

	// Print on a printer
	if *printFlag {
		fmt.Println("true")
		// Printout(data)
	}

	return nil
}

// Output file names that cause some error
func PrintErrorFilesList() {
	fmt.Println("/**************** FILES THAT FAILED TO GET ****************/")
	for _, v := range errorFileNames {
		fmt.Println(v)
	}
	fmt.Println("/**********************************************************/")
}

func main() {
	// Parse flag
	printFlag = flag.Bool("p", false, "Printout spreadsheets.")
	flag.Parse()
	errorFileNames = make([]string, 0, 16)
	inputFolderId()

	if err := os.Mkdir(dist, 0777); err != nil {
		fmt.Printf("Unable to create directory: %v\n", err)
	}

	b, err := ioutil.ReadFile(credentials)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, scopes)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)
	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	// Output Google Folder Id Name
	r, err := srv.Files.Get(folderId).Do()
	if err != nil {
		log.Fatalf("Does not exist the folder")
	}
	fmt.Printf("Folder Name: %s\n", r.Name)

	// Main
	pageToken := ""
	errorFlag := false
	succ := 0
	fail := 0
	for {
		q := srv.Files.List().PageSize(500).
			Fields("nextPageToken, files(parents, id, name, mimeType, trashed)")
		if pageToken != "" {
			q = q.PageToken(pageToken)
		}
		r, err := q.Do()
		if err != nil {
			log.Fatalf("Unable to retrieve files: %v", err)
		}

		for _, i := range r.Files {
			if !i.Trashed && contains(i.Parents, folderId) >= 0 {
				err := FromSpreadsheetToPdf(i, config)
				// err := DownloadFile(srv, i.Id, dist+"/"+i.Name+".pdf")
				// err := PrintFile (srv, i.Id)
				fmt.Printf("%s\t%s\x1b[0m\n", i.Id, i.Name)
				if err != nil {
					errorFlag = true
					fail++
				} else {
					succ++
				}
			}
		}

		// When there is no next page token, break loop
		pageToken = r.NextPageToken
		if pageToken == "" {
			break
		}
	}

	fmt.Printf("Succeeded %d files, failed %d files.\n", succ, fail)

	if errorFlag {
		PrintErrorFilesList()
	}
}
