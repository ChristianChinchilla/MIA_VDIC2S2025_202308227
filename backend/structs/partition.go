package structs

type Partition struct {
    Part_status      byte     
    Part_type        byte     
    Part_fit         byte     
    Part_start       int64    
    Part_s           int64    
    Part_name        [16]byte 
    Part_correlative int64    
    Part_id          [4]byte  
}

func NewPartition(status byte, p_type byte, fit byte, start int64, s int64, name [16]byte) Partition {
    var partition Partition

    partition.Part_status = status
    partition.Part_type = p_type
    partition.Part_fit = fit
    partition.Part_start = start
    partition.Part_s = s
    partition.Part_name = name
    partition.Part_correlative = -1

    return partition
}