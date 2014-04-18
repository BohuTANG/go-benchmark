package main

import (
	"flag"
	"github.com/Tokutek/tokubenchmark"
	"github.com/Tokutek/tokubenchmark/mongotools"
	"labix.org/v2/mgo"
	"log"
	"math/rand"
	"time"
)

func main() {
	// needed for making/accessing collections:
	host := flag.String("host", "localhost", "host:port string of database to connect to")
	dbname := flag.String("db", "iibench", "dbname")
	collname := flag.String("coll", "purchases_index", "collname")
	numCollections := flag.Int("numCollections", 1, "number of collections to simultaneously run on")

	// doc generator specific variables:
	numCharFields := flag.Int("numCharFields", 0, "specify the number of additional char fields stored in an array")
	charFieldLength := flag.Int("charFieldLength", 5, "specify length of char fields")

	// for IIBenchQuery
	queryResultLimit := flag.Int("queryResultLimit", 10, "number of results queries should be limited to")
	queriesPerInterval := flag.Uint64("queriesPerInterval", 100, "max queries per interval, 0 means unlimited")
	queryInterval := flag.Uint64("queryInterval", 1, "interval for queries, in seconds, meant to be used with -queriesPerInterval")

	// for benchmark
	numWriters := flag.Int("numWriterThreads", 1, "specify the number of writer threads")
	numQueryThreads := flag.Int("numQueryThreads", 0, "specify the number of threads to perform queries")
	numSeconds := flag.Int64("numSeconds", 5, "number of seconds the benchmark is to run. If this value is > 0, then numInsertsPerThread MUST be 0, and vice versa")
	numInsertsPerThread := flag.Uint64("numInsertsPerThread", 0, "number of inserts to be done per thread. If this value is > 0, then numSeconds MUST be 0 and numQueryThreads MUST be 0")

	flag.Parse()

	if *numInsertsPerThread > 0 && (*numQueryThreads > 0 || *numSeconds > 0) {
		log.Fatal("Invalid values for numInsertsPerThread: ", *numInsertsPerThread, ", numQueryThreads: ", *numQueryThreads, ", numSeconds: ", *numSeconds)
	}

	session, err := mgo.Dial(*host)
	if err != nil {
		log.Fatal("Error connecting to ", *host, ": ", err)
	}
	// so we are not in fire and forget
	session.SetSafe(&mgo.Safe{})
	defer session.Close()

	indexes := make([]mgo.Index, 3)
	indexes[0] = mgo.Index{Key: []string{"pr", "cid"}}
	indexes[1] = mgo.Index{Key: []string{"crid", "pr", "cid"}}
	indexes[2] = mgo.Index{Key: []string{"pr", "ts", "cid"}}

	mongotools.MakeCollections(*collname, *dbname, *numCollections, session, indexes)
	// at this point we have created the collection, now run the benchmark
	res := new(mongotools.IIBenchResult)
	workers := make([]tokubenchmark.BenchmarkWorkInfo, 0, *numWriters+*numQueryThreads)
	for i := 0; i < *numWriters; i++ {
		var gen *mongotools.IIBenchDocGenerator = new(mongotools.IIBenchDocGenerator)
		// we want each worker to have it's own random number generator
		// because generating random numbers takes a mutex
		gen.RandSource = rand.New(rand.NewSource(time.Now().UnixNano()))
		gen.CharFieldLength = *charFieldLength
		gen.NumCharFields = *numCharFields
		currCollectionString := mongotools.GetCollectionString(*collname, i%*numCollections)
		workers = append(workers, mongotools.MakeCollectionWriter(gen, session, *dbname, currCollectionString, *numInsertsPerThread))
	}
	for i := 0; i < *numQueryThreads; i++ {
		currCollectionString := mongotools.GetCollectionString(*collname, i%*numCollections)
		copiedSession := session.Copy()
		copiedSession.SetSafe(&mgo.Safe{})
		query := mongotools.IIBenchQuery{
			copiedSession,
			*dbname,
			currCollectionString,
			rand.New(rand.NewSource(time.Now().UnixNano())),
			time.Now(),
			*queryResultLimit,
			0}
		workInfo := tokubenchmark.BenchmarkWorkInfo{query, *queriesPerInterval, *queryInterval, 0}
		workers = append(workers, workInfo)
	}
	tokubenchmark.RunBenchmark(res, workers, time.Duration(*numSeconds)*time.Second)
}