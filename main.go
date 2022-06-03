package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	proxies, err := readFile("proxies.txt")
	if err != nil {
		log.Fatal(err)
	}

	manager, err := NewManager(proxies)

	ids, err := readFile("ids.txt")
	if err != nil {
		log.Fatal(err)
	}

	result, err := os.Create("result.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer result.Close()

	for _, id := range ids {
		id = strings.Replace(id, "|", "", -1)
		id = strings.TrimSpace(id)

		phone, err := manager.GetPhone(id)
		if err != nil {
			if e, ok := err.(*BlockedError); ok {
				log.Fatalf("worker blocked: %v", e)
			}
			log.Fatal(err)
		}

		_, err = result.Write([]byte(fmt.Sprintf("%s,%s\n", id, phone)))
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s,%s\n", id, phone)
	}
}

func readFile(file string) ([]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		out = append(out, scanner.Text())
	}

	return out, nil
}
