package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/square/keysync/backup"
	"golang.org/x/crypto/nacl/box"

	"gopkg.in/alecthomas/kingpin.v2"
)

var b64 = base64.StdEncoding

func main() {
	var (
		app            = kingpin.New("keyunwrap", "A tool to unwrap a keysync backup key")
		privateKeyFile = app.Flag("privatekeyfile", "The offline private key").Required().String()
		generateCmd    = app.Command("generate", "Generate a new key pair")
		unwrapCmd      = app.Command("unwrap", "Unwrap a backup key")
		publicKeyFile  = generateCmd.Flag("publickeyfile", "Generate public key here").Required().String()
		wrappedKey     = unwrapCmd.Flag("wrapped", "The wrapped backup key").Required().ExistingFile()
	)

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case generateCmd.FullCommand():
		if err := generateKey(*privateKeyFile, *publicKeyFile); err != nil {
			log.Fatal(err.Error())
		}
	case unwrapCmd.FullCommand():
		key, err := unwrap(*privateKeyFile, *wrappedKey)
		if err != nil {
			log.Fatal(err.Error())
		}
		// In the success case we just print the key out
		fmt.Println(b64.EncodeToString(key))
	}
}

// unwrap reads files and Unwraps.
func unwrap(privateKeyFile, wrappedKey string) ([]byte, error) {
	privKey, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("error reading private key: %v", err)
	}

	wrapped, err := ioutil.ReadFile(wrappedKey)
	if err != nil {
		return nil, fmt.Errorf("error reading wrapped key: %v", err)
	}

	return backup.Unwrap(wrapped, privKey)
}

func generateKey(privateKeyFile, publicKeyFile string) error {
	if info, err := os.Stat(privateKeyFile); !os.IsNotExist(err) {
		return fmt.Errorf("expected private key to not exist, instead got %v: %v", info, err)
	}

	pubkey, privkey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	// Base64 encode the pubkey to make it "friendlier" for putting in configuration yamls
	b64pubkey := make([]byte, b64.EncodedLen(len(pubkey)))
	b64.Encode(b64pubkey, pubkey[:])

	if err := ioutil.WriteFile(publicKeyFile, b64pubkey, 0444); err != nil {
		return fmt.Errorf("error writing private key: %v", err)
	}

	// Don't base64 the private key to make it harder to confuse with the public key.
	if err := ioutil.WriteFile(privateKeyFile, privkey[:], 0400); err != nil {
		return fmt.Errorf("error writing private key: %v", err)
	}

	return nil
}
