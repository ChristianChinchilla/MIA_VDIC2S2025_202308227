package structs


type EBR struct {
	PartMount byte
	PartFit   byte
	PartStart int64
	PartS    int64
	PartNext  int64
	PartName  [16]byte

}