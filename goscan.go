package main

import (
	"bytes"
	"debug/buildinfo"
	"debug/elf"
	"debug/gosym"
	"encoding/binary"
	"fmt"
	"strings"
)

// from https://gitlab.cee.redhat.com/dbenoit/gosyms-example/-/blob/master/gosyms-example.go

// From go/src/debug/gosym/pclntab.go
const (
	go12magic  = 0xfffffffb
	go116magic = 0xfffffffa
	go118magic = 0xfffffff0
	go120magic = 0xfffffff1
)

// Print all function names defined in .gopclntab
func printFuncs(symTable *gosym.Table) {
	for _, f := range symTable.Funcs {
		fmt.Printf("%s\n", f.Name)
	}
}

// Select the magic number based on the Go version
func magicNumber(fileName, goVersion string) []byte {
	bs := make([]byte, 4)
	var magic uint32
	if strings.Compare(goVersion, "go1.20") >= 0 {
		magic = go120magic
	} else if strings.Compare(goVersion, "go1.18") >= 0 {
		magic = go118magic
	} else if strings.Compare(goVersion, "go1.16") >= 0 {
		magic = go116magic
	} else {
		magic = go12magic
	}
	binary.LittleEndian.PutUint32(bs, magic)
	return bs
}

// Construct a hashmap of build settings
func buildSettingMap(bi *buildinfo.BuildInfo) map[string]string {
	settings := make(map[string]string)
	for _, bs := range bi.Settings {
		settings[bs.Key] = bs.Value
	}
	return settings
}

// Open a Go ELF executable and read .gopclntab
func readTable(fileName string) (*gosym.Table, error) {
	// Default section label is .gopclntab
	sectionLabel := ".gopclntab"
	bi, err := buildinfo.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	// If built with PIE and stripped, gopclntab is
	// unlabeled and nested under .data.rel.ro.
	settings := buildSettingMap(bi)
	if settings["-buildmode"] == "pie" {
		sectionLabel = ".data.rel.ro"
	}

	exe, err := elf.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer exe.Close()

	section := exe.Section(sectionLabel)
	if section == nil {
		return nil, fmt.Errorf("could not read section .gopclntab from %s ", fileName)
	}
	tableData, err := section.Data()
	if err != nil {
		return nil, fmt.Errorf("found section but could not read .gopclntab from %s ", fileName)
	}

	// Find .gopclntab by magic number even if there is no section label
	magic := magicNumber(fileName, bi.GoVersion)
	pclntabIndex := bytes.Index(tableData, magic)
	if pclntabIndex < 0 {
		return nil, fmt.Errorf("could not find magic number in %s ", fileName)
	}
	tableData = tableData[pclntabIndex:]
	addr := exe.Section(".text").Addr
	lineTable := gosym.NewLineTable(tableData, addr)
	symTable, err := gosym.NewTable([]byte{}, lineTable)
	if err != nil {
		return nil, fmt.Errorf("could not create symbol table from  %s ", fileName)
	}
	return symTable, nil
}

// Checks the .gopclntab for the expected list of symbols.
func ExpectedSyms(expectedSyms []string, symTable *gosym.Table) error {
	for _, s := range expectedSyms {
		fn := symTable.LookupFunc(s)
		if fn == nil {
			return fmt.Errorf("symbol %v not found", s)
		}
	}
	return nil
}
