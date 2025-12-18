package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

func ExecuteMkfs(id string, formatType string) {
	//normalizar el tipo de formateo
	formatType = strings.ToLower(formatType)
	if formatType == "" {
		formatType = "full"
	}

	if formatType != "full" {
		fmt.Printf("Error: Tipo de formateo '%s' no soportado. Use 'full'.\n", formatType)
		return
	}

	fmt.Printf("Iniciando formateo %s de la partición...\n", strings.ToUpper(formatType))

	//buscar la particion montada por id
	mounted := GetMountedPartition(id)
	if mounted == nil {
		fmt.Printf("Error: No se encontró ninguna partición montada con ID '%s'.\n", id)
		return
	}

	//debug informacion de la particion montada
	fmt.Printf("Partición montada encontrada:\n")
	fmt.Printf("   ID: %s\n", mounted.ID)
	fmt.Printf("   Nombre: '%s'\n", mounted.Name)
	fmt.Printf("   Ruta: %s\n", mounted.Path)
	fmt.Printf("   Tamaño: %d bytes\n", mounted.Size)

	//abrir el archivo del disco
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Printf("Error al abrir el disco: %v\n", err)
		return
	}
	defer file.Close()

	//leer el mbr para obtener informacion de la particion
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al leer el MBR: %v\n", err)
		return
	}

	//encontrar la particion especifica
	var partition *structs.Partition
	for _, p := range mbr.Mbr_partitions {
		if p.Part_status != '0' {
			partitionNameBytes := p.Part_name[:]
			nullIndex := len(partitionNameBytes)
			for j, b := range partitionNameBytes {
				if b == 0 {
					nullIndex = j
					break
				}
			}
			partitionName := strings.TrimSpace(string(partitionNameBytes[:nullIndex]))

			//usar comparacion mas flexible
			if strings.EqualFold(partitionName, mounted.Name) || partitionName == mounted.Name {
				partitionCopy := p
				partition = &partitionCopy
				break
			}
		}
	}

	if partition == nil {
		fmt.Printf("Error: No se pudo encontrar la partición '%s'.\n", mounted.Name)
		fmt.Printf("🔍 Particiones disponibles en el MBR:\n")
		for i, p := range mbr.Mbr_partitions {
			if p.Part_status != '0' {
				partitionNameBytes := p.Part_name[:]
				nullIndex := len(partitionNameBytes)
				for j, b := range partitionNameBytes {
					if b == 0 {
						nullIndex = j
						break
					}
				}
				partitionName := strings.TrimSpace(string(partitionNameBytes[:nullIndex]))
				fmt.Printf("   [%d] '%s' (status: %c, type: %c)\n", i, partitionName, p.Part_status, p.Part_type)
			}
		}
		return
	}

	//calcular el numero de estructuras segun la formula ext2
	n := calculateEXT2Structures(partition.Part_s)

	fmt.Printf("Calculando estructuras EXT2 para partición de %d bytes...\n", partition.Part_s)
	fmt.Printf("   - Número de inodos: %d\n", n)
	fmt.Printf("   - Número de bloques: %d\n", n*3)

	//crear el superbloque
	superblock := createSuperblock(n, partition.Part_s, partition.Part_start)

	//escribir las estructuras ext2 en la particion
	if err := writeEXT2Structures(file, partition, superblock, n); err != nil {
		fmt.Printf("Error al escribir estructuras EXT2: %v\n", err)
		return
	}

	//crear archivo users.txt en la raiz
	if err := createUsersFile(file, &superblock); err != nil {
		fmt.Printf("Error al crear archivo users.txt: %v\n", err)
		return
	}

	fmt.Printf("Sistema de archivos EXT2 creado exitosamente en partición '%s'.\n", mounted.Name)
	fmt.Printf("   ID: %s\n", id)
	fmt.Printf("   Tipo: %s\n", strings.ToUpper(formatType))
	fmt.Printf("   Inodos: %d\n", n)
	fmt.Printf("   Bloques: %d\n", n*3)
	fmt.Printf("   Archivo users.txt creado en la raíz\n")
}

// crear archivo users.txt en la raiz
func createUsersFile(file *os.File, superblock *structs.SuperBloque) error {
	//contenido inicial del archivo users.txt
	usersContent := "1,G,root\n1,U,root,root,123\n"
	inodeIndex := int64(1)

	//crear inodo para el archivo users.txt
	now := time.Now().Unix()
	fileInode := structs.Inodos{
		I_uid:   0,
		I_gid:   0,
		I_s:     int64(len(usersContent)),
		I_atime: now,
		I_ctime: now,
		I_mtime: now,
		I_type:  '1',
		I_perm:  [3]byte{'6', '4', '4'},
	}

	//el primer bloque apunta al bloque 1
	fileInode.I_block[0] = 1

	//inicializar el resto de bloques en -1
	for i := 1; i < 15; i++ {
		fileInode.I_block[i] = -1
	}

	//debug posiciones
	inodePosition := superblock.S_inode_start + (inodeIndex * superblock.S_inode_s)
	blockPosition := superblock.S_block_start + (1 * superblock.S_block_s)

	//escribir el inodo
	file.Seek(inodePosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &fileInode); err != nil {
		return fmt.Errorf("error escribiendo inodo users.txt: %v", err)
	}

	//crear bloque de archivo con el contenido
	fileBlock := structs.BloqueArchivo{}

	//limpiar todo el bloque
	for i := range fileBlock.BContent {
		fileBlock.BContent[i] = 0
	}

	//copiar el contenido
	copy(fileBlock.BContent[:], []byte(usersContent))

	//escribir el bloque
	file.Seek(blockPosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &fileBlock); err != nil {
		return fmt.Errorf("error escribiendo bloque users.txt: %v", err)
	}

	//actualizar el directorio raiz
	if err := addFileToRootDirectory(file, superblock, "users.txt", inodeIndex); err != nil {
		return fmt.Errorf("error agregando users.txt al directorio raiz: %v", err)
	}

	//actualizar bitmaps
	if err := updateBitmaps(file, superblock, inodeIndex, 1); err != nil {
		return fmt.Errorf("error actualizando bitmaps: %v", err)
	}

	//actualizar contadores del superbloque
	superblock.S_free_inodes_count--
	superblock.S_free_blocks_count--
	superblock.S_first_ino = 2
	superblock.S_first_blo = 2

	return nil
}

// agregar archivo al directorio raiz
func addFileToRootDirectory(file *os.File, superblock *structs.SuperBloque, fileName string, inodeIndex int64) error {
	file.Seek(superblock.S_block_start, 0)

	var rootBlock structs.BloqueCarpeta
	if err := binary.Read(file, binary.LittleEndian, &rootBlock); err != nil {
		return err
	}

	//validar longitud del nombre
	if len(fileName) > 12 {
		return fmt.Errorf("nombre de archivo demasiado largo maximo 12 caracteres")
	}

	//limpiar la entrada en posicion 2
	for j := range rootBlock.BContent[2].BName {
		rootBlock.BContent[2].BName[j] = 0
	}

	//copiar el nombre del archivo
	copy(rootBlock.BContent[2].BName[:], []byte(fileName))
	rootBlock.BContent[2].BInodo = inodeIndex

	fmt.Printf("Archivo '%s' agregado en posicion 2\n", fileName)

	//escribir el bloque actualizado
	file.Seek(superblock.S_block_start, 0)
	return binary.Write(file, binary.LittleEndian, &rootBlock)
}

// actualizar bitmaps
func updateBitmaps(file *os.File, superblock *structs.SuperBloque, inodeIndex int64, blockIndex int64) error {
	file.Seek(superblock.S_bm_inode_start+inodeIndex, 0)
	if _, err := file.Write([]byte{1}); err != nil {
		return err
	}

	//actualizar bitmap de bloques
	file.Seek(superblock.S_bm_block_start+blockIndex, 0)
	if _, err := file.Write([]byte{1}); err != nil {
		return err
	}

	return nil
}

// calcular numero de estructuras segun ext2
func calculateEXT2Structures(partitionSize int64) int64 {
	superblockSize := int64(binary.Size(structs.SuperBloque{}))
	inodeSize := int64(binary.Size(structs.Inodos{}))

	carpetaSize := int64(binary.Size(structs.BloqueCarpeta{}))
	archivoSize := int64(binary.Size(structs.BloqueArchivo{}))

	var blockSize int64
	if carpetaSize > archivoSize {
		blockSize = carpetaSize
	} else {
		blockSize = archivoSize
	}

	numerator := float64(partitionSize - superblockSize)
	denominator := float64(1 + 3 + inodeSize + 3*blockSize)

	n := math.Floor(numerator / denominator)

	if n <= 0 {
		n = 1
	}

	return int64(n)
}

// crear superbloque con valores iniciales
func createSuperblock(inodeCount int64, _ int64, partitionStart int64) structs.SuperBloque {
	blockCount := inodeCount * 3
	now := time.Now().Unix()

	//calcular posiciones de las estructuras
	superblockSize := int64(binary.Size(structs.SuperBloque{}))
	bitmapInodesStart := partitionStart + superblockSize
	bitmapBlocksStart := bitmapInodesStart + inodeCount
	inodesStart := bitmapBlocksStart + blockCount

	carpetaSize := int64(binary.Size(structs.BloqueCarpeta{}))
	archivoSize := int64(binary.Size(structs.BloqueArchivo{}))

	var blockRealSize int64
	if carpetaSize > archivoSize {
		blockRealSize = carpetaSize
	} else {
		blockRealSize = archivoSize
	}

	blocksStart := inodesStart + inodeCount*int64(binary.Size(structs.Inodos{}))

	return structs.SuperBloque{
		S_file_system_type:  2,
		S_inodes_count:      inodeCount,
		S_blocks_count:      blockCount,
		S_free_blocks_count: blockCount - 2,
		S_free_inodes_count: inodeCount - 2,
		S_mtime:             now,
		S_umtime:            now,
		S_mnt_count:         1,
		S_magic:             0xEF53,
		S_inode_s:           int64(binary.Size(structs.Inodos{})),
		S_block_s:           blockRealSize,
		S_first_ino:         2,
		S_first_blo:         2,
		S_bm_inode_start:    bitmapInodesStart,
		S_bm_block_start:    bitmapBlocksStart,
		S_inode_start:       inodesStart,
		S_block_start:       blocksStart,
	}
}

// escribir todas las estructuras ext2
func writeEXT2Structures(file *os.File, partition *structs.Partition, superblock structs.SuperBloque, n int64) error {
	file.Seek(partition.Part_start, 0)

	//escribir superbloque
	if err := binary.Write(file, binary.LittleEndian, &superblock); err != nil {
		return fmt.Errorf("error escribiendo superbloque: %v", err)
	}

	//bitmap de inodos
	bitmapInodes := make([]byte, n)
	bitmapInodes[0] = 1
	if _, err := file.Write(bitmapInodes); err != nil {
		return fmt.Errorf("error escribiendo bitmap de inodos: %v", err)
	}

	//bitmap de bloques
	bitmapBlocks := make([]byte, n*3)
	bitmapBlocks[0] = 1
	if _, err := file.Write(bitmapBlocks); err != nil {
		return fmt.Errorf("error escribiendo bitmap de bloques: %v", err)
	}

	//escribir inodo raiz
	rootInode := createRootInode()
	if err := binary.Write(file, binary.LittleEndian, &rootInode); err != nil {
		return fmt.Errorf("error escribiendo inodo raiz: %v", err)
	}

	emptyInode := structs.Inodos{}
	for i := 0; i < 15; i++ {
		emptyInode.I_block[i] = -1
	}

	for i := int64(1); i < n; i++ {
		if err := binary.Write(file, binary.LittleEndian, &emptyInode); err != nil {
			return fmt.Errorf("error escribiendo inodos vacios: %v", err)
		}
	}

	//escribir bloque raiz
	rootBlock := createRootBlock()
	if err := binary.Write(file, binary.LittleEndian, &rootBlock); err != nil {
		return fmt.Errorf("error escribiendo bloque raiz: %v", err)
	}

	emptyBlock := structs.BloqueArchivo{}
	for i := range emptyBlock.BContent {
		emptyBlock.BContent[i] = 0
	}

	for i := int64(1); i < n*3; i++ {
		if err := binary.Write(file, binary.LittleEndian, &emptyBlock); err != nil {
			return fmt.Errorf("error escribiendo bloques vacios: %v", err)
		}
	}

	return nil
}

// crear el inodo raiz
func createRootInode() structs.Inodos {
	now := time.Now().Unix()

	inode := structs.Inodos{
		I_uid:   0,
		I_gid:   0,
		I_s:     0,
		I_atime: now,
		I_ctime: now,
		I_mtime: now,
		I_type:  '0',
		I_perm:  [3]byte{'7', '5', '5'},
	}

	inode.I_block[0] = 0

	for i := 1; i < 15; i++ {
		inode.I_block[i] = -1
	}

	return inode
}

// crear el bloque raiz
func createRootBlock() structs.BloqueCarpeta {
	var block structs.BloqueCarpeta

	for i := 0; i < 4; i++ {
		for j := range block.BContent[i].BName {
			block.BContent[i].BName[j] = 0
		}
		block.BContent[i].BInodo = -1
	}

	copy(block.BContent[0].BName[:], []byte("."))
	block.BContent[0].BInodo = 0

	copy(block.BContent[1].BName[:], []byte(".."))
	block.BContent[1].BInodo = 0

	return block
}
