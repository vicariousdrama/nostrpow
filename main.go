package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/nbd-wtf/go-nostr"
	"go.uber.org/atomic"

	"mleku.online/git/interrupt"
	"mleku.online/git/qu"
)

type Result struct {
	routinenumber int
	nonce         string
	highestpow    int
}

func main() {
	var e error
	p := message.NewPrinter(language.English)

	// setup logging
	LOG_FILE := "nostrpow.log"
	logFile, e := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if e != nil {
		log.Panic(e)
	}
	defer logFile.Close()
	consoleOut := os.Stdout
	multiout := io.MultiWriter(consoleOut, logFile)
	log.SetOutput(multiout)

	// Number of goroutines that will run
	routineCount := runtime.NumCPU() - 2
	if len(os.Args) > 4 {
		routineCount, e = strconv.Atoi(os.Args[4])
		if e != nil {
			panic(e)
		}
	}
	if routineCount < 1 {
		routineCount = 1
	}

	// Batch Size
	batchSize := 100_000_000_000 // 100 billion
	if len(os.Args) > 5 {
		batchSize, e = strconv.Atoi(os.Args[5])
		if e != nil {
			panic(e)
		}
	}
	routineSize := batchSize / routineCount
	if (batchSize % routineCount) > 0 {
		routineSize++
	}

	// Input file
	filename := "nostrevent.json"
	if len(os.Args) > 1 {
		filename = os.Args[1]
	}
	byteValue, _ := os.ReadFile(filename)
	var nostrevent nostr.Event
	json.Unmarshal(byteValue, &nostrevent)

	// POW level and starting nonce value
	pow := 24
	nonceval := 0
	nonceTagIndex := -1

	if nostrevent.Tags == nil {
		nostrevent.Tags = make(nostr.Tags, 0)
	}
	for i, t := range nostrevent.Tags {
		if t[0] == "nonce" {
			nonceTagIndex = i
		}
	}
	if nonceTagIndex > -1 {
		nonceval, e = strconv.Atoi(nostrevent.Tags[nonceTagIndex][1])
		if e != nil {
			panic(e)
		}
		pow, e = strconv.Atoi(nostrevent.Tags[nonceTagIndex][2])
		if e != nil {
			panic(e)
		}
	} else {
		// BUG IS HERE FOR INITIALIZING A NONCE TAG
		var nonceTags nostr.Tags
		var nonceTag nostr.Tag = make([]string, 3)
		nonceTag[0] = "nonce"
		nonceTag[1] = strconv.Itoa(nonceval)
		nonceTag[2] = strconv.Itoa(pow)
		nonceTags = append(nonceTags, nonceTag)
		nostrevent.Tags.AppendUnique(nonceTag)
		nonceTagIndex = len(nostrevent.Tags) - 1
	}
	if len(os.Args) > 2 {
		pow, e = strconv.Atoi(os.Args[2])
		if e != nil {
			panic(e)
		}
		nostrevent.Tags[nonceTagIndex][2] = strconv.Itoa(pow)
	}
	if len(os.Args) > 3 {
		nonceval, e = strconv.Atoi(os.Args[3])
		if e != nil {
			panic(e)
		}
		nostrevent.Tags[nonceTagIndex][1] = strconv.Itoa(nonceval)
	}

	nsizewc := p.Sprintf("%d", nonceval)
	rsizewc := p.Sprintf("%d", routineSize)
	bsizewc := p.Sprintf("%d", batchSize)
	log.Printf("Starting %d routines (%s checks each) in batch of %s starting at %s\n", routineCount, rsizewc, bsizewc, nsizewc)

	started := time.Now()
	quit, shutdown := qu.T(), qu.T()
	resC := make(chan Result)
	interrupt.AddHandler(func() {
		// this will stop work if CTRL-C or Interrupt signal from OS.
		log.Printf("CTRL-C or interrupt signal received. Shutting down\n")
		log.Printf("==================================================\n")
		shutdown.Q()
	})
	var wg sync.WaitGroup
	wgcounter := atomic.NewInt64(0)
	counter := atomic.NewInt64(0)
	for i := 0; i < routineCount; i++ {
		routineEvent := nostrevent
		var routineTags nostr.Tags
		for _, t := range nostrevent.Tags {
			l := len(t)
			var routineTag nostr.Tag = make([]string, l)
			for k, v := range t {
				routineTag[k] = v
			}
			routineTags = append(routineTags, routineTag)
		}
		routineEvent.Tags = routineTags
		//routineTags := nostrevent.Tags
		//routineEvent.Tags = routineTags
		indexStart := nonceval + (i * routineSize)
		indexEnd := indexStart + routineSize
		go dopow(i, routineEvent, indexStart, indexEnd, nonceTagIndex, quit, resC, &wg, counter, wgcounter)
		time.Sleep(200 * time.Millisecond)
	}
	tick := time.NewTicker(time.Second * 10)
	var res Result
	wgcheck := false

out:
	for {
		select {
		case <-tick.C:
			workingFor := time.Now().Sub(started)
			wm := workingFor % time.Second
			workingFor -= wm
			withCommaThousandSep := p.Sprintf("%d", counter.Load())
			log.Printf("working for %12v, attempts %s\n",
				workingFor, withCommaThousandSep)
			wgcheck = true
		case r := <-resC:
			// one of the workers found the solution
			res = r
			// tell the others to stop
			quit.Q()
			break out
		case <-shutdown.Wait():
			quit.Q()
			//			log.I.Ln("\rinterrupt signal received")
			os.Exit(0)
		}
		if wgcheck && wgcounter.Load() < 1 {
			quit.Q()
			break out
		}
	}

	// wait for all of the workers to stop
	wg.Wait()

	if res.highestpow == pow {
		log.Printf("nonce found: %s\n", res.nonce)
		withCommaThousandSep := p.Sprintf("%d", counter.Load())
		log.Printf("found in %s attempts using %d threads, taking %v\n",
			withCommaThousandSep, routineCount, time.Now().Sub(started))
		log.Printf("routine %d found nonce %s with pow %d\n", res.routinenumber, res.nonce, res.highestpow)
	} else {
		log.Printf("no solution found for range %d to %d\n", nonceval, nonceval+batchSize)
	}
}

func HexToBin(hex string) (string, error) {
	hex2 := hex[0:16]
	ui, err := strconv.ParseUint(hex2, 16, 64)
	if err != nil {
		return "", err
	}

	// %064b indicates base 2, zero padded, with 64 characters
	return fmt.Sprintf("%064b", ui), nil
}

func dopow(routinenumber int, routineevent nostr.Event, indexStart int, indexEnd int, nonceTagIndex int, quit qu.C, resC chan Result, wg *sync.WaitGroup, counter *atomic.Int64, wgcounter *atomic.Int64) {
	wg.Add(1)
	wgcounter.Inc()
	log.Printf("routine %d checking nonces from %d to %d\n", routinenumber, indexStart, indexEnd)
	log.Printf("for %v \n", routineevent)
	var r Result
	var e error
	found := false
	index := indexStart - 1
	powtofind, e := strconv.Atoi(routineevent.Tags[nonceTagIndex][2])
	if e != nil {
		panic(e)
	}
	var curpow int
	var highestpow int
	var highestpownonce string
	tick := time.NewTicker(time.Second * 180)

out:
	for index < indexEnd {
		index++
		select {
		case <-tick.C:
			log.Printf("routine %d highest pow seen so far: %d (nonce %s). now trying nonce %d\n", routinenumber, highestpow, highestpownonce, index)
		case <-quit:
			wg.Done()
			wgcounter.Dec()
			if found {
				// send back the result
				log.Printf("routine %d sending back result\n", routinenumber)
				resC <- r
				log.Printf("sent\n")
			} else {
				log.Printf("routine %d quit received from other thread\n", routinenumber)
			}
			break out
		default:
		}
		counter.Inc()

		routineevent.Tags[nonceTagIndex][1] = strconv.Itoa(index)
		//log.Println(nostrevent.Tags)
		eventID := routineevent.GetID()
		bin, e := HexToBin(eventID[0:16]) // limit to 8 bytes worth, or first 64 bits
		if e != nil {
			panic(e)
		}
		curpow = strings.Index(bin, "1")
		if curpow == powtofind {
			r.routinenumber = routinenumber
			r.highestpow = powtofind
			r.nonce = routineevent.Tags[nonceTagIndex][1]
			found = true
			quit.Q()
		} else {
			if curpow > highestpow {
				highestpow = curpow
				highestpownonce = routineevent.Tags[nonceTagIndex][1]
			}
		}
	}

	if !found && index >= indexEnd {
		log.Printf("routine %d reached end of checks for this routine (%d to %d)\n", routinenumber, indexStart, indexEnd)
		wgcounter.Dec()
		wg.Done()
	}

}
