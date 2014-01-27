/*
  the github.com/smizoe/get_imgs/main package collects images
  based on the given query.
*/

package main

import (
	"flag"
	"github.com/smizoe/get_imgs/modules"
	"log"
	"os"
)

func main() {
	var (
		outputDir   string
		maxRoutines int
		query       string
		accKey      string
	)

	flag.StringVar(&outputDir, "output-dir", os.TempDir(), "Specifies the directory to store images.")
	flag.IntVar(&maxRoutines, "max-routines", 4, "Specifies the max number of go routines to be spawned to download images.")
	flag.StringVar(&query, "query", "", "Specifies the query string to be searched.")
	flag.StringVar(&accKey, "access-key", "", "Specifies the key to query against Bing API.")

	flag.Parse()
	logInfo := log.New(os.Stderr, "I: ", log.Flags())
	logError := log.New(os.Stderr, "E: ", log.Flags())

	if len(query) == 0 {
		os.Stderr.WriteString("Youm ust give a query by --query option! Abort.")
		os.Exit(1)
	}

	opStatus := make(chan *modules.OperationReply)
	chanPair := make(chan *modules.ResultPair)

	req := modules.NewRequester(opStatus, query, accKey, chanPair)
	spa := modules.NewSpawner(opStatus, chanPair, outputDir, maxRoutines)
	go req.Main()
	go spa.Main()
	logInfo.Print("Main process started.")

	reply, ok := <-opStatus
	for ok {
		if reply.Status == modules.Success {
			logInfo.Print(reply.ObjType, ": Succeeded.")
		} else {
			logError.Print(reply.ObjType, ": ", reply.ErrorMsg)
		}
		reply, ok = <-opStatus
	}
}
