package main

import (
	"fmt"
	//"github.com/paulsmith/gogeos/geos"
	"github.com/vetinari/osm"
	"github.com/vetinari/osm/pbf"
	//"github.com/vetinari/osm/xml"
	"encoding/csv"
	"flag"
	s2 "github.com/golang/geo/s2"
	"github.com/vetinari/osm/node"
	"github.com/vetinari/osm/relation"
	"github.com/vetinari/osm/way"
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
	/*
		strInput := "LINESTRING (0 0, 10 10, 20 20)"
		line, _ := geos.FromWKT(strInput)
		lineBuf, _ := line.Buffer(2.5)
		strLineBuf, _ := lineBuf.ToWKT()
		fmt.Sprintf("The buffered geom is %s", strLineBuf)
		log.Println(buf)
	*/
	// Output: POLYGON ((18.2322330470336311 21.7677669529663689, 18.61…
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
	//log.Println(osmExtract.String())

	// INTERESSANT --> osmExtract.FilterTags()

	//generateS2cells(ways)
	//s2ways := generateS2cells(ways)
	//wayS2CellGenerator()
	//readWays
	//getWayID
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
	/*
		if len(os.Args) != 2 {
			fmt.Fprintf(os.Stderr, "%s: Usage: %s PBF_FILE\n", os.Args[0], os.Args[0])
			os.Exit(1)
		}
	*/
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
	//reader.Read() // Skip Header
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

	var regionCoverer s2.RegionCoverer
	regionCoverer.MaxCells = 10000
	regionCoverer.MinLevel = 17 // ~50m segment length
	regionCoverer.MaxLevel = 17 // ~50m segment length
	cuRect := cu.RectBound()
	cuCells := regionCoverer.Covering(cuRect) // get cells covered at level17

	fmt.Println("Processing ways...")
	for _, way := range *ways {
		//add := false
		for _, node := range way.GetNodes() {
			// Generate s2cell for Node
			lat := node.Position().Lat
			lng := node.Position().Lon
			s2LatLng := s2.LatLngFromDegrees(lat, lng)
			s2PointCellID := s2.CellIDFromLatLng(s2LatLng)
			//log.Printf("[%f,%f] - %v - #%s \n", lat, lng, s2LatLng, s2PointCellID.ToToken())

			if cuCells.IntersectsCellID(s2PointCellID) {
				wayID := way.Id()
				// if wayID == 299131213 {
				// 	fmt.Printf("%v \n", osmFile.GetWay(wayID))
				// }
				waysExtract[wayID] = osmFile.GetWay(wayID)
				for _, wayNode := range way.GetNodes() {
					// if wayID == 299131213 {
					// 	fmt.Printf("%v \n", osmFile.GetNode(wayNode.Id()))
					// }
					nodesExtract[wayNode.Id()] = osmFile.GetNode(wayNode.Id())
					//fmt.Printf("Added way #%d - with nodes %v \n", way.Id(), way.GetNodes())
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

	/*
		for _, node := range nodes {
			// Generate s2cell for Node
			lat := node.Position().Lat
			lng := node.Position().Lon
			s2LatLng := s2.LatLngFromDegrees(lat, lng)
			s2PointCellID := s2.CellIDFromLatLng(s2LatLng)
		}
	*/
	/*
		for _, way := range *ways {

			nodes := way.GetNodes()
			for _, node := range nodes {
				// Generate s2cell for Node
				lat := node.Position().Lat
				lng := node.Position().Lon
				s2LatLng := s2.LatLngFromDegrees(lat, lng)
				s2PointCellID := s2.CellIDFromLatLng(s2LatLng)
				//log.Printf("[%f,%f] - %v - #%s \n", lat, lng, s2LatLng, s2PointCellID.ToToken())

				// check if CellUnion contains nodeCell
				if cu.IntersectsCellID(s2PointCellID) {
					add = true
					break
				}
			}

		}
		for _, relation := range *relations {
			relationWays := relation.GetWays()

		}
	*/
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

/*
	var regionCoverer s2.RegionCoverer

	regionCoverer.MaxCells = 10000
	regionCoverer.MinLevel = 7
	regionCoverer.MaxLevel = 15

	rect := loop.RectBound()
	if loop.NumEdges() == 74767 { // damn you Canada!
		log.Printf("Canada has always been a special one...")
		topleft := s2.LatLngFromDegrees(72.0000064, -141.0000000)
		topright := s2.LatLngFromDegrees(72.0000064, -55.6152420)
		bottomleft := s2.LatLngFromDegrees(41.9017143-10.0, -141.0000000)
		bottomright := s2.LatLngFromDegrees(41.9017143-10.0, -55.6152420)

		rect = s2.Rect{}
		rect = rect.AddPoint(bottomleft)
		rect = rect.AddPoint(bottomright)
		rect = rect.AddPoint(topright)
		rect = rect.AddPoint(topleft)
	}

	covering := regionCoverer.Covering(s2.Region(rect))
*/
