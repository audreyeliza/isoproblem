package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kdomanski/iso9660/util"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <iso-file-path>")
	}
	isoPath := os.Args[1]

	//check if file exists
	if _, err := os.Stat(isoPath); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("File does not exist: %s", isoPath)
	}

	if filepath.Ext(isoPath) != ".iso" {
		log.Printf("Warning: File does not end with '.iso'. This may not be a standard ISO file.")
	}

	//create unique output directory
	outerDir := "parsed_iso"
	isoName := filepath.Base(isoPath)
	isoName = isoName[:len(isoName)-len(filepath.Ext(isoName))]
	timestamp := time.Now().Format("20060102-150405")
	innerDir := fmt.Sprintf("%s_extracted_%s", isoName, timestamp)
	targetDir := filepath.Join(outerDir, innerDir)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	//open iso file
	isoFile, err := os.Open(isoPath)
	if err != nil {
		log.Fatalf("Failed to open iso file: %v", err)
	}
	defer isoFile.Close()

	//extract iso file
	if err := util.ExtractImageToDirectory(isoFile, targetDir); err != nil {
		log.Fatalf("Failed to extract image: %v", err)
	}

	//list all extracted files
	fmt.Println("Extracted files:")
	err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fmt.Println(path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to walk directory: %v", err)
	}

	fmt.Printf("All files extracted to: %s\n", targetDir)
}
