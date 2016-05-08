package main

import (
	"flag"
	"fmt"
	"github.com/antonholmquist/jason"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	infileFlag   = flag.String("infile", "", "Path to input JSON schema")
	outfileFlag  = flag.String("outfile", "", "Path to output go file")
)

func writeNestedRecords(outfile *os.File, record *jason.Object) {
	// get info about Record
	name, err := record.GetString("name")
	if err != nil || len(name) == 0 {
		log.Fatal("Record lacks a name: ", record)
	}
	namespace, err := record.GetString("namespace")
	if err != nil || len(namespace) == 0 {
		log.Fatal("Record lacks a namespace: ", record)
	}

	// write Record contents
	if _, err = fmt.Fprintf(outfile, "	%sNamespace = `%s`\n", name, namespace); err != nil {
		log.Fatalf("Couldn't write namespace %s to %s: %s", namespace, *outfileFlag, err)
	}
	if _, err = fmt.Fprintf(outfile, "	%sSchema = `%s`\n\n", name, record.String()); err != nil {
		log.Fatalf("Couldn't write var %s to %s: %s", name, *outfileFlag, err)
	}

	// search for nested Records. let's assume that Records are only nested in Arrays.
	recordFields, err := record.GetObjectArray("fields")
	if err != nil {
		log.Fatalf("Expected fields in record %s", name)
	}
	for i, field := range recordFields {
		log.Printf("Scanning %s['fields'][%d]", name, i)
		fieldType, err := field.GetObject("type")
		if err != nil {
			continue // 'type' is not an object
		}
		fieldTypeType, err := fieldType.GetString("type")
		if err != nil {
			log.Fatalf("Expected %s['fields'][%d]['type']['type'] == str", name, i)
		}
		if fieldTypeType != "array" {
			log.Fatalf("Expected %s['fields'][%d]['type']['type'] == 'array'", name, i)
		}
		fieldTypeItems, err := fieldType.GetObject("items")
		if err != nil {
			log.Fatalf("Expected %s['fields'][%d]['type']['items'] == object", name, i)
		}
		writeNestedRecords(outfile, fieldTypeItems) // recurse
	}
}

func gitRev() string {
	rev, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		fmt.Fprint(os.Stderr, "Failed to get Git revision: ", err)
		return "UNKNOWN"
	} else {
		sanitizedRev := strings.TrimSpace(string(rev))
		log.Print("Git revision: ", sanitizedRev)
		return sanitizedRev
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr,
			"Generates Go source file with a named var containing an Avro JSON schema\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *infileFlag == "" {
		flag.Usage()
		log.Fatalf("Missing argument: -infile")
	}
	log.Printf("Opening JSON %s\n", *infileFlag)
	infile, err := os.Open(*infileFlag)
	if err != nil {
		log.Fatalf("Couldn't open input %s: %s", *infileFlag, err)
	}

	data, err := ioutil.ReadAll(infile)
	if err != nil {
		log.Fatalf("Failed to read from %s: %s", *infileFlag, err)
	}
	// prune "doc" entries from the schema by hacking them out of the file directly
	// (we do this because 'jason' lib doesn't support editing JSON properly
	prunedData := make([]byte, 0)
	for i, row := range strings.Split(string(data), "\n") {
		if strings.Contains(row, "\"doc\"") {
			log.Printf("Skipping doc (row %d): %s\n", i, strings.TrimSpace(row))
		} else {
			prunedData = append(prunedData, []byte(row + "\n")...)
		}
	}

	rootObject, err := jason.NewObjectFromBytes(prunedData)
	if err != nil {
		log.Fatalf("Failed to parse JSON from %s: %s", *infileFlag, err)
	}

	// check output flags before creating an output file:
	if *outfileFlag == "" {
		flag.Usage()
		log.Fatalf("Missing argument: -outfile")
	}

	outfile, err := os.Create(*outfileFlag)
	if err != nil {
		log.Fatalf("Couldn't open output %s: %s", *outfileFlag, err)
	}
	if _, err = fmt.Fprint(outfile, `package collector

// THIS FILE IS AUTOGENERATED BY 'go generate'. DO NOT EDIT.
// goavro requires that we extract each nested message in our schema. so here we are.

`); err != nil {
		log.Fatalf("Couldn't write header to %s: %s", *outfileFlag, err)
	}
	if _, err = fmt.Fprintf(outfile, "// Generated at: %s\n", time.Now().String()); err != nil {
		log.Fatalf("Couldn't write timestamp to %s: %s", *outfileFlag, err)
	}
	if _, err = fmt.Fprintf(outfile, "// Command: %+v\n", strings.Join(os.Args, " ")); err != nil {
		log.Fatalf("Couldn't write git rev to %s: %s", *outfileFlag, err)
	}
	if _, err = fmt.Fprintf(outfile, "// Git revision: %s\n\n", gitRev()); err != nil {
		log.Fatalf("Couldn't write git rev to %s: %s", *outfileFlag, err)
	}
	if _, err = fmt.Fprint(outfile, "const (\n\n"); err != nil {
		log.Fatalf("Couldn't write varintro to %s: %s", *outfileFlag, err)
	}

	writeNestedRecords(outfile, rootObject)

	if _, err = fmt.Fprint(outfile, `)

// AGAIN, THIS FILE IS AUTOGENERATED BY 'go generate'. DO NOT EDIT.
`); err != nil {
		log.Fatalf("Couldn't write footer to %s: %s", *outfileFlag, err)
	}
}
