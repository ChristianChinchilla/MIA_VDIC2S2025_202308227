package structs

import (
	"time"
)

type MBR struct {
	Mbr_tamano         int64
	Mbr_fecha_creacion int64
	Mbr_dsk_signature  int64
	Dsk_fit            byte
	Mbr_partitions     [4]Partition
}

func NewMBR(size int64, fit byte, signature int64) MBR {
	var mbr MBR

	mbr.Mbr_tamano = size
	mbr.Mbr_fecha_creacion = time.Now().Unix()
	mbr.Mbr_dsk_signature = signature
	mbr.Dsk_fit = fit

	for i := 0; i < 4; i++ {
		mbr.Mbr_partitions[i].Part_status = '0'
		mbr.Mbr_partitions[i].Part_start = -1
	}

	return mbr
}
