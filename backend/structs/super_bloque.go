package structs

type SuperBloque struct {
    S_file_system_type   int64
    S_inodes_count      int64
    S_blocks_count      int64
    S_free_blocks_count int64
    S_free_inodes_count int64
    S_mtime            int64
    S_umtime           int64
    S_mnt_count        int64
    S_magic            int64
    S_inode_s          int64
    S_block_s          int64
    S_first_ino        int64
    S_first_blo        int64
    S_bm_inode_start   int64
    S_bm_block_start   int64
    S_inode_start      int64
    S_block_start      int64
}