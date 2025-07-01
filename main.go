package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sectorSize = 2048

type DirEntry struct {
	Name     string
	IsDir    bool
	Extent   uint32
	Size     uint32      //in bytes
	Children []*DirEntry //slice of pointers, read pointer article
}

// both-endian handling from kdom iso9660_databases.go
func UnmarshalUint32LSBMSB(data []byte) (uint32, error) { //look into unmarshaling
	if len(data) < 8 {
		return 0, fmt.Errorf("insufficient data for uint32")
	}
	lsb := binary.LittleEndian.Uint32(data[0:4])
	msb := binary.BigEndian.Uint32(data[4:8])
	if lsb != msb {
		return 0, fmt.Errorf("endians don't match: %d != %d", lsb, msb)
	}
	return lsb, nil
}

func UnmarshalUint16LSBMSB(data []byte) (uint16, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("insufficient data for uint16")
	}
	lsb := binary.LittleEndian.Uint16(data[0:2])
	msb := binary.BigEndian.Uint16(data[2:4])
	if lsb != msb {
		return 0, fmt.Errorf("endians don't match: %d != %d", lsb, msb)
	}
	return lsb, nil
}

func parseDirectory(iso *os.File, extent uint32, size uint32) ([]*DirEntry, error) {
	dirData := make([]byte, size)
	_, err := iso.Seek(int64(extent)*sectorSize, 0)
	check(err)
	_, err = iso.Read(dirData)
	check(err)

	var entries []*DirEntry
	offset := 0
	for offset < len(dirData) {
		if offset >= len(dirData) {
			break
		}
		recLen := int(dirData[offset]) //rec for record
		if recLen == 0 {
			nextSector := ((offset / sectorSize) + 1) * sectorSize
			if nextSector <= offset {
				break
			}
			offset = nextSector
			continue
		}
		if offset+recLen > len(dirData) {
			break
		}
		rec := dirData[offset : offset+recLen]

		extent, err := UnmarshalUint32LSBMSB(rec[2:10]) //future me the numbers are in the os dev page or ecma-119 standard
		if err != nil {
			return nil, err
		}
		size, err := UnmarshalUint32LSBMSB(rec[10:18])
		if err != nil {
			return nil, err
		}
		fileFlags := rec[25]
		fileIdLen := int(rec[32])
		fileId := string(rec[33 : 33+fileIdLen])
		cleanName := strings.Split(fileId, ";")[0]

		//check ur notes for that thing about starting with . and .. and something about extra padding?
		if fileId == "\x00" || fileId == "\x01" {
			offset += recLen
			continue
		}

		entry := &DirEntry{
			Name:   cleanName,
			IsDir:  fileFlags&0x02 != 0,
			Extent: extent,
			Size:   size,
		}
		entries = append(entries, entry)
		offset += recLen
	}
	return entries, nil
}

// recommended root dir recurse, future search path table or just get better??
func extractAll(iso *os.File, entries []*DirEntry, outDir string) (uint64, error) {
	var totalBytes uint64
	for _, entry := range entries {
		outPath := filepath.Join(outDir, entry.Name)
		if entry.IsDir {
			if err := os.MkdirAll(outPath, 0755); err != nil { //notes about access number add meanings
				return totalBytes, err
			}
			children, err := parseDirectory(iso, entry.Extent, entry.Size)
			if err != nil {
				return totalBytes, err
			}
			childBytes, err := extractAll(iso, children, outPath)
			if err != nil {
				return totalBytes, err
			}
			totalBytes += childBytes
		} else {
			_, err := iso.Seek(int64(entry.Extent)*sectorSize, 0)
			if err != nil {
				return totalBytes, err
			}
			data := make([]byte, entry.Size)
			_, err = iso.Read(data)
			if err != nil {
				return totalBytes, err
			}
			if err := os.WriteFile(outPath, data, 0644); err != nil {
				return totalBytes, err
			}
			fmt.Printf("Extracted: %s (%d bytes)\n", outPath, entry.Size)
			totalBytes += uint64(entry.Size)
		}
	}
	return totalBytes, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <input.iso>\n", os.Args[0])
		os.Exit(1)
	}
	isoPath := os.Args[1]

	fileInfo, err := os.Stat(isoPath)
	check(err)
	isoSizeBytes := fileInfo.Size()
	isoSizeGigs := float64(isoSizeBytes) / (1024 * 1024 * 1024) //perhaps get better at abbreviation

	baseName := strings.TrimSuffix(filepath.Base(isoPath), filepath.Ext(isoPath))
	timestamp := time.Now().Format("20060102_150405") //save timestamp though i should probs know why it's important
	outDir := fmt.Sprintf("%s_extracted_%s", baseName, timestamp)

	if err := os.MkdirAll(outDir, 0755); err != nil {
		panic(err)
	}

	iso, err := os.Open(isoPath)
	check(err)
	defer iso.Close()

	//pvd sector 16
	_, err = iso.Seek(16*sectorSize, 0)
	check(err)
	pvd := make([]byte, sectorSize)
	_, err = iso.Read(pvd)
	check(err)

	//root dir record offset 156, 34 bytes long
	rootRec := pvd[156 : 156+34]
	rootExtent, err := UnmarshalUint32LSBMSB(rootRec[2:10]) //table is in the long boring ecma file
	check(err)
	rootSize, err := UnmarshalUint32LSBMSB(rootRec[10:18])
	check(err) //shoutout to the highlight of the weekend which was figuring out the endianness and directory stuff, only to immediately have my dreams crushed

	rootEntries, err := parseDirectory(iso, rootExtent, rootSize)
	check(err)

	totalBytes, err := extractAll(iso, rootEntries, outDir)
	check(err)

	totalGigs := float64(totalBytes) / (1024 * 1024 * 1024) // byte conversion chart
	extractionRatio := float64(totalBytes) / float64(isoSizeBytes)

	fmt.Printf("\nExtraction Summary\n")
	fmt.Printf("Original ISO size: %.3f GB (%d bytes)\n", isoSizeGigs, isoSizeBytes)
	fmt.Printf("Total extracted size: %.3f GB (%d bytes)\n", totalGigs, totalBytes)
	fmt.Printf("Extraction ratio: %.4f (%.2f%%)\n", extractionRatio, extractionRatio*100)
	fmt.Printf("Output folder: %s\n", outDir)
}

func check(err error) { //probs find a better error handler than from that one youtube tutorial
	if err != nil {
		panic(err)
	}
}

//notes for self!!!
//rn can only do level 1 and 2 cause of single extent
//level 3 up to 8 TiB, which is...a lot i think it's just 4 TiB right now?
//basically im getting more than what's actually in there so theres probably some multi extent stuff i need to look into?
//only alternative search the path table? something about corrupted path files though
//also organize this cause i'm starting to lose where i am in here and maybe make it cleaner rather than a tutorial amalgamation
