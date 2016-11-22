package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/brechtvm/osm"
	"github.com/brechtvm/osm/node"
	"github.com/brechtvm/osm/pbf"
	"github.com/brechtvm/osm/relation"
	"github.com/brechtvm/osm/way"
	s2 "github.com/golang/geo/s2"
	"io"
	"log"
	"os"
	"strings"
)

var s2ways map[uint64][]uint64
var inputFile string
var cellUnionFile string

func main() {
	parseFlags()
	log.Println("Reading osm file")
	osmFile := readOSMPbf(inputFile)
	log.Println("OsmFile Read")
	log.Println("Reading cellUnion")
	cellUnion := readCellUnion(cellUnionFile)
	log.Printf("CellUnion Read [%d] \n", len(cellUnion))
	log.Println("Extract OSM data by CellUnion")
	osmExtract := extractByCellUnion(osmFile, cellUnion)
	log.Println("Done extracting OSM data")
	writeOsm(osmExtract.String(), "extract.osm")
}

func parseFlags() {
	var inputFileFlag = flag.String("input", "", "input-file (.osm.pbf)")
	var cellUnionFileFlag = flag.String("cellUnion", "", "s2 cellUnion (.csv)")

	flag.Parse()

	if *inputFileFlag == "" {
		log.Fatal("Parameter input not given.")
	}
	inputFile = *inputFileFlag

	if *cellUnionFileFlag == "" {
		log.Fatal("Parameter cellUnion not given.")
	} else {
		cellUnionFile = *cellUnionFileFlag
	}
}

func readOSMPbf(path string) *osm.OSM {
	fh, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: failed to open %s: %s\n", os.Args[0], os.Args[1], err)
		os.Exit(1)
	}
	defer fh.Close()

	o, err := osm.New(pbf.Parser(fh))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: failed to parse %s: %s\n", os.Args[0], os.Args[1], err)
		os.Exit(1)
	}
	log.Printf("ways [%d] - nodes [%d] - relations [%d]\n", o.GetWayList().Len(), o.GetNodeList().Len(), o.GetRelationList().Len())
	return o
}

func readCellUnion(path string) s2.CellUnion {
	var cu s2.CellUnion
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: failed to open %s: %s\n", os.Args[0], os.Args[1], err)
		os.Exit(1)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = ';'
	lineCount := 0
	for {
		// read just one record, but we could ReadAll() as well
		record, err := reader.Read()
		// end-of-file is fitted into err
		if err == io.EOF {
			log.Println("end of file")
			break
		} else if err != nil {
			log.Println(fmt.Sprintf("Error at line %d:", lineCount), err)
			return nil
		}
		cellID := s2.CellIDFromToken(record[0])
		cu = append(cu, cellID)
	}
	return cu
}

// Extract OSM file by a given S2 CellUnion
func extractByCellUnion(osmFile *osm.OSM, cu s2.CellUnion) osm.OSM {
	ways := osmFile.GetWayList()
	relations := osmFile.GetRelationList()

	osmExtract := *osm.NewOSM()
	osmExtract.Ways = nil
	osmExtract.Nodes = nil

	waysExtract := make(map[int64]*way.Way)
	nodesExtract := make(map[int64]*node.Node)
	relationsExtract := make(map[int64]*relation.Relation)
	log.Printf("CellUnion #%d \n", len(cu))

	fmt.Println("Processing ways...")
	for _, way := range *ways {
		//add := false
		for _, node := range way.GetNodes() {
			// Generate s2cell for Node
			lat := node.Position().Lat
			lng := node.Position().Lon
			s2LatLng := s2.LatLngFromDegrees(lat, lng)
			s2PointCellID := s2.CellIDFromLatLng(s2LatLng)
			if cu.IntersectsCellID(s2PointCellID) {
				wayID := way.Id()
				waysExtract[wayID] = osmFile.GetWay(wayID)
				for _, wayNode := range way.GetNodes() {
					nodesExtract[wayNode.Id()] = osmFile.GetNode(wayNode.Id())
				}
				break
			}
		}
	}
	fmt.Println("Processing relations...")
	for _, relation := range *relations {
		ways := relation.GetWays()
		for _, way := range ways {
			if way == nil {
				break
			}
			if waysExtract[way.Id()] != nil {
				relationsExtract[relation.Id()] = relation
			}
		}
	}
	osmExtract.Ways = waysExtract
	osmExtract.Nodes = nodesExtract
	osmExtract.Relations = relationsExtract

	log.Printf("ways [%d] - nodes [%d] - relations [%d]\n", osmExtract.GetWayList().Len(), osmExtract.GetNodeList().Len(), osmExtract.GetRelationList().Len())

	return osmExtract
}

func writeOsm(osm string, filename string) {
	f, err := os.Create(strings.Join([]string{"./", filename}, ""))
	check(err)
	defer f.Close()
	f.WriteString(osm)
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
