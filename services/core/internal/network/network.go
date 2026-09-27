// Package network loads the metro topology and provides the synthetic demand
// model shared (formula-for-formula) with the Python AI service.
// Keep demand.go and ai/metronav_ai/demand.py in sync.
package network

import (
	"encoding/json"
	"fmt"
	"os"
)

type Station struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	X                float64 `json:"x"`
	Y                float64 `json:"y"`
	Weight           float64 `json:"weight"`
	PlatformCapacity int     `json:"platform_capacity"`
}

type Line struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Color         string    `json:"color"`
	Coaches       int       `json:"coaches"`
	CoachCapacity int       `json:"coach_capacity"`
	HeadwayMin    float64   `json:"headway_min"`
	Stations      []string  `json:"stations"`
	SegmentMin    []float64 `json:"segment_min"`
}

type FareSlab struct {
	MaxStations int `json:"max_stations"`
	Fare        int `json:"fare"`
}

type Network struct {
	Note        string             `json:"_note"`
	Stations    []Station          `json:"stations"`
	Lines       []Line             `json:"lines"`
	TransferMin map[string]float64 `json:"transfer_min"`
	FareSlabs   []FareSlab         `json:"fare_slabs"`

	stationByID map[string]*Station
	lineByID    map[string]*Line
}

func Load(path string) (*Network, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read network: %w", err)
	}
	return Parse(b)
}

func Parse(b []byte) (*Network, error) {
	var n Network
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, fmt.Errorf("parse network: %w", err)
	}
	if err := n.index(); err != nil {
		return nil, err
	}
	return &n, nil
}

func (n *Network) index() error {
	n.stationByID = map[string]*Station{}
	for i := range n.Stations {
		n.stationByID[n.Stations[i].ID] = &n.Stations[i]
	}
	n.lineByID = map[string]*Line{}
	for i := range n.Lines {
		l := &n.Lines[i]
		if len(l.SegmentMin) != len(l.Stations)-1 {
			return fmt.Errorf("line %s: %d stations but %d segments", l.ID, len(l.Stations), len(l.SegmentMin))
		}
		for _, s := range l.Stations {
			if _, ok := n.stationByID[s]; !ok {
				return fmt.Errorf("line %s references unknown station %s", l.ID, s)
			}
		}
		n.lineByID[l.ID] = l
	}
	return nil
}

func (n *Network) Station(id string) (*Station, bool) { s, ok := n.stationByID[id]; return s, ok }
func (n *Network) Line(id string) (*Line, bool)       { l, ok := n.lineByID[id]; return l, ok }

// LinesAt returns IDs of lines serving a station.
func (n *Network) LinesAt(stationID string) []string {
	var out []string
	for _, l := range n.Lines {
		if l.Index(stationID) >= 0 {
			out = append(out, l.ID)
		}
	}
	return out
}

// RunTime is the one-way end-to-end running time of a line in minutes.
func (l *Line) RunTime() float64 {
	t := 0.0
	for _, s := range l.SegmentMin {
		t += s
	}
	return t
}

func (l *Line) Index(stationID string) int {
	for i, s := range l.Stations {
		if s == stationID {
			return i
		}
	}
	return -1
}

// Fare applies the configured slab table to a count of stations travelled.
func (n *Network) Fare(stationsTravelled int) int {
	for _, s := range n.FareSlabs {
		if stationsTravelled <= s.MaxStations {
			return s.Fare
		}
	}
	if len(n.FareSlabs) > 0 {
		return n.FareSlabs[len(n.FareSlabs)-1].Fare
	}
	return 0
}
