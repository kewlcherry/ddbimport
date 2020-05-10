package main

import (
	"encoding/csv"
	"flag"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/a-h/ddbimport/csvtodynamo"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

var regionFlag = flag.String("region", "", "The AWS region where the DynamoDB table is located")
var tableFlag = flag.String("table", "", "The DynamoDB table name to import to.")
var csvFlag = flag.String("csv", "", "The CSV file to upload to DynamoDB.")
var numericFieldsFlag = flag.String("numericFields", "", "A comma separated list of fields that are numeric.")
var booleanFieldsFlag = flag.String("booleanFields", "", "A comma separated list of fields that are boolean.")
var delimiterFlag = flag.String("delimiter", "comma", "The delimiter of the CSV file - 'tab' or 'comma'")

func main() {
	flag.Parse()
	if *regionFlag == "" || *tableFlag == "" || *csvFlag == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("importing %q to %s in region %s", *csvFlag, *tableFlag, *regionFlag)

	start := time.Now()
	var duration time.Duration

	sess, err := session.NewSession(&aws.Config{Region: aws.String(*regionFlag)})
	if err != nil {
		log.Fatalf("failed to create session: %v", err)
	}
	client := dynamodb.New(sess)

	f, err := os.Open(*csvFlag)
	if err != nil {
		log.Fatalf("failed to open CSV file: %v", err)
	}
	defer f.Close()

	csvr := csv.NewReader(f)
	csvr.Comma = delimiter(*delimiterFlag)
	conf := csvtodynamo.NewConfiguration()
	conf.AddNumberKeys(strings.Split(*numericFieldsFlag, ",")...)
	conf.AddBoolKeys(strings.Split(*booleanFieldsFlag, ",")...)
	reader, err := csvtodynamo.NewConverter(csvr, conf)
	if err != nil {
		log.Fatalf("failed to create CSV reader: %v", err)
	}

	var batchCount, totalCount int
	for {
		batchCount++
		batch, read, err := reader.ReadBatch()
		if err != nil && err != io.EOF {
			log.Fatalf("failed to read batch %d: %v", batchCount, err)
		}
		end := err == io.EOF
		totalCount += read
		err = BatchPut(client, *tableFlag, batch[:read])
		if err != nil {
			log.Fatalf("error executing batch put: %v", err)
			return
		}
		if batchCount%100 == 0 {
			duration = time.Since(start)
			log.Printf("%d records per second", int(float64(totalCount)/duration.Seconds()))
		}
		if end {
			break
		}
	}
	duration = time.Since(start)
	log.Printf("%d records per second", int(float64(totalCount)/duration.Seconds()))
	log.Print("complete")
}

func delimiter(s string) rune {
	if s == "," || s == "\t" {
		return rune(s[0])
	}
	if s == "tab" {
		return '\t'
	}
	return ','
}

func BatchPut(client *dynamodb.DynamoDB, tableName string, records []map[string]*dynamodb.AttributeValue) (err error) {
	writeRequests := make([]*dynamodb.WriteRequest, len(records))
	for i := 0; i < len(records); i++ {
		writeRequests[i] = &dynamodb.WriteRequest{
			PutRequest: &dynamodb.PutRequest{
				Item: records[i],
			},
		}
	}
	_, err = client.BatchWriteItem(&dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]*dynamodb.WriteRequest{
			tableName: writeRequests,
		},
	})
	return
}
