package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"crawlab.org/internal/crypto"
)

func main() {
	crypto.InitKey("")

	switch os.Args[1] {
	case "enc":
		raw := os.Args[2]
		encrypt, err := crypto.Encrypt([]byte(raw))
		if err != nil {
			fmt.Println("Err:", err)
		} else {
			fmt.Println(base64.StdEncoding.EncodeToString(encrypt))
		}
	case "dec":
		raw := os.Args[2]
		body, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			fmt.Println("Err:", err)
		} else {
			plain, err := crypto.Decrypt(body)
			if err != nil {
				fmt.Println("Err:", err)
			} else {
				fmt.Println(string(plain))
			}
		}
	}

}
