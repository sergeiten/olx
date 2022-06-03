package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const authURL = "https://www.olx.uz/api/open/oauth/token/"
const phoneURL = "https://www.olx.uz/api/v1/offers/%s/limited-phones/"
const detailURL = "https://www.olx.uz/d/nedvizhimost/"

type BlockedError struct {
	status int
}

func (s *BlockedError) Error() string {
	return fmt.Sprintf("worker blocked, response status %d", s.status)
}

type requestTokenParams struct {
	DeviceID     string `json:"device_id"`
	DeviceToken  string `json:"device_token"`
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type responseToken struct {
	AccessToken string `json:"access_token"`
}

type responsePhone struct {
	Data map[string][]string
}

type Worker struct {
	client      *C
	accessToken string
	ua          []string
}

// 1   get client id and client secret
// 1.1 make get request to any item detail page
// 1.2 there is window.__INIT_CONFIG__ variable inside source code which contains client_id and client_secret values
// 1.3 parse reponse and get values

// 2   generate device_id
// 2.1 generate random uint8 array with 16 elements
// 2.2 change some elements a[6] = 15 & a[6] | 64, a[8] = 63 & a[8] | 128
// 2.3 generate string array from 0 to 255 with values of (r + 256).toString(16).substr(1)
// 2.4 get value from array (2.3) where a key is value from array (2.2) which is start from 0
//     which should has the following format
//     [0,1,2,3]-[4,5]-[7,8]-[9,10]-[11,12,13,14,15,16]

// 3   build device_token
// 3.1 make string {"id":"deviceGUID"}
// 3.2 encode by base64 from 2.2
// 3.3 make hash from 2.3 by using HMAC with digest algorithm SHA1 and secret key "device"
// 3.4 make string splited by dot from 2.3 and 2.4

// 4 get authorization bearer token
// make post request to get bearer token
// https://www.olx.uz/api/open/oauth/token/
// {
//   "device_id": "845ecfa5-7c6f-4856-a75b-5be51720d28b",
//   "device_token": "eyJpZCI6Ijg0NWVjZmE1LTdjNmYtNDg1Ni1hNzViLTViZTUxNzIwZDI4YiJ9.9fc3c8a27b90f500d91f3ff54351c7e5846c62b1",
//   "grant_type": "device",
//   "client_id": "100309",
//   "client_secret": "QVnzW1SoFUt0JoNJmiBvMsKWkFvG9NUKZCdrjegVlZYCc8FR"
// }
func NewWorker(client *C) (*Worker, error) {
	w := &Worker{
		client: client,
		ua:     make([]string, 0),
	}

	err := w.init()

	return w, err
}

func (s *Worker) init() error {
	f, err := os.Open("agents.txt")
	if err != nil {
		return err
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	for scan.Scan() {
		ua := scan.Text()
		ua = strings.TrimSpace(ua)
		ua = strings.Trim(ua, "\n")
		ua = strings.Trim(ua, "\r")

		s.ua = append(s.ua, ua)
	}

	resp, err := s.client.Get(detailURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	dat, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	clientID, clientSecret, err := s.getClientIDAndSecret(string(dat))
	if err != nil {
		return err
	}

	deviceID := s.generateDeviceID()

	deviceToken := s.generateDeviceToken(deviceID)

	fmt.Printf("client_id: %s, client_secret: %s\n", clientID, clientSecret)
	fmt.Printf("device_id: %s\n", deviceID)
	fmt.Printf("device_token: %s\n", deviceToken)

	values := requestTokenParams{
		DeviceID:     deviceID,
		DeviceToken:  deviceToken,
		GrantType:    "device",
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	body, err := json.Marshal(values)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, authURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))

	resp, err = s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var rspValues responseToken
	err = json.NewDecoder(resp.Body).Decode(&rspValues)
	if err != nil {
		return err
	}

	s.accessToken = rspValues.AccessToken

	return nil
}

func (s *Worker) GetPhone(id string) (string, error) {
	url := fmt.Sprintf(phoneURL, id)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	r := rand.New(rand.NewSource(time.Now().UnixMicro()))
	ua := s.ua[r.Intn(len(s.ua))]

	req.Header.Set("Authorization", "Bearer "+s.accessToken)
	req.Header.Set("User-Agent", ua)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// it seems IP blocked
	if resp.StatusCode == 403 {
		dat, _ := ioutil.ReadAll(resp.Body)
		println(string(dat))

		return "", &BlockedError{
			status: resp.StatusCode,
		}
	}

	if resp.StatusCode == 400 {
		return "", nil
	}

	fmt.Printf("%+v\n", resp.Status)

	var rspPhone responsePhone
	err = json.NewDecoder(resp.Body).Decode(&rspPhone)
	if err != nil {
		return "", err
	}

	if len(rspPhone.Data["phones"]) > 0 {
		return rspPhone.Data["phones"][0], nil
	}

	return "", nil
}

// 1   get client id and client secret
// 1.1 make get request to any item detail page
// 1.2 there is window.__INIT_CONFIG__ variable inside source code which contains client_id and client_secret values
// 1.3 parse reponse and get values
func (s *Worker) getClientIDAndSecret(body string) (string, string, error) {
	r := regexp.MustCompile(`client_id\\\":\\\"([0-9]+).+\"client_secret\\\":\\\"([A-z0-9]+)\\\"`)

	found := r.FindStringSubmatch(body)
	if len(found) == 3 {
		return found[1], found[2], nil
	}

	return "", "", errors.New("can not find client id and secret")
}

// 2   generate device_id
// 2.1 generate random uint8 array with 16 elements
// 2.2 change some elements a[6] = 15 & a[6] | 64, a[8] = 63 & a[8] | 128
// 2.3 generate string array from 0 to 255 with values of (r + 256).toString(16).substr(1)
// 2.4 get value from array (2.3) where a key is value from array (2.2) which is start from 0
//     which should has the following format
//     [0,1,2,3]-[4,5]-[6,7]-[8,9]-[10,11,12,13,14,15]
func (s *Worker) generateDeviceID() string {
	r := rand.New(rand.NewSource(time.Now().UnixMicro()))

	rndUint8 := make([]uint8, 16)

	for i := 0; i < len(rndUint8); i++ {
		rndUint8[i] = uint8(r.Intn(255))
	}

	// magic staff
	rndUint8[6] = 15&rndUint8[6] | 64
	rndUint8[8] = 63&rndUint8[8] | 128

	hexes := make([]string, 256)
	for i := 0; i < len(hexes); i++ {
		v := fmt.Sprintf("%x", (i + 256))

		hexes[i] = v[(len(v) - 2):]
	}

	// [0,1,2,3]-[4,5]-[6,7]-[8,9]-[10,11,12,13,14,15]
	var deviceID string
	for i := 0; i < 16; i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			deviceID += "-"
		}

		deviceID += hexes[rndUint8[i]]
	}

	return deviceID
}

// 3   build device_token
// 3.1 make string {"id":"deviceGUID"}
// 3.2 encode string by base64 from previous step
// 3.3 make hash from previous step by using HMAC with digest algorithm SHA1 and secret key "device"
// 3.4 convert hash to hex
// 3.5 make string splited by dot from 3.2 and 3.4 (base64.hex)
func (s *Worker) generateDeviceToken(deviceID string) string {
	value := fmt.Sprintf(`{"id":"%s"}`, deviceID)

	encoded := base64.StdEncoding.EncodeToString([]byte(value))

	mac := hmac.New(sha1.New, []byte("device"))
	mac.Write([]byte(encoded))
	hash := mac.Sum(nil)

	hex := fmt.Sprintf("%x", hash)

	return fmt.Sprintf("%s.%s", encoded, hex)
}
