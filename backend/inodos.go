package structs

type Inodos struct {
    I_uid   int64    
    I_gid   int64    
    I_s     int64    
    I_atime int64    
    I_ctime int64    
    I_mtime int64    
    I_block [15]int64
    I_type  byte     
    I_perm  [3]byte  
}