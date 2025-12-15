package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

// Leer el contenido completo de cualquier archivo (multi-bloque)
func ReadFileContent(mounted *MountedPartition, fileName string) (string, error) {
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return "", fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	// Obtener partición y superbloque
	_, superblock, err := getPartitionAndSuperblock(file, mounted)
	if err != nil {
		return "", err
	}

	// Buscar el archivo en el directorio raíz
	inodeIndex, err := findFileInRootDirectory(file, superblock, fileName)
	if err != nil {
		return "", fmt.Errorf("archivo '%s' no encontrado: %v", fileName, err)
	}

	// Leer el inodo del archivo
	inodePosition := superblock.S_inode_start + (inodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var fileInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &fileInode); err != nil {
		return "", fmt.Errorf("error al leer el inodo de '%s': %v", fileName, err)
	}

	// Leer el contenido completo (multi-bloque)
	content, err := readFileContentMultiBlock(file, superblock, &fileInode)
	if err != nil {
		return "", fmt.Errorf("error al leer el contenido de '%s': %v", fileName, err)
	}

	return content, nil
}

// Escribir contenido completo a cualquier archivo (multi-bloque)
func WriteFileContent(mounted *MountedPartition, fileName, newContent string) error {
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	// Obtener partición y superbloque
	_, superblock, err := getPartitionAndSuperblock(file, mounted)
	if err != nil {
		return err
	}

	// Buscar el archivo en el directorio raíz
	inodeIndex, err := findFileInRootDirectory(file, superblock, fileName)
	if err != nil {
		return fmt.Errorf("archivo '%s' no encontrado: %v", fileName, err)
	}

	// Leer el inodo del archivo
	inodePosition := superblock.S_inode_start + (inodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var fileInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &fileInode); err != nil {
		return fmt.Errorf("error al leer el inodo de '%s': %v", fileName, err)
	}

	// Escribir contenido multi-bloque
	err = writeFileContentMultiBlock(file, superblock, &fileInode, newContent, inodePosition)
	if err != nil {
		return fmt.Errorf("error al escribir '%s': %v", fileName, err)
	}

	return nil
}

func ReadUsersFileContent(mounted *MountedPartition) (string, error) {
	return ReadFileContent(mounted, "users.txt")
}

func WriteUsersFileContent(mounted *MountedPartition, newContent string) error {
	return WriteFileContent(mounted, "users.txt", newContent)
}

// Obtener partición y superbloque
func getPartitionAndSuperblock(file *os.File, mounted *MountedPartition) (*structs.Partition, *structs.SuperBloque, error) {
	// Leer el MBR
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		return nil, nil, fmt.Errorf("error al leer el MBR: %v", err)
	}

	// Encontrar la partición específica
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

			if strings.EqualFold(partitionName, mounted.Name) || partitionName == mounted.Name {
				partitionCopy := p
				partition = &partitionCopy
				break
			}
		}
	}

	if partition == nil {
		return nil, nil, fmt.Errorf("no se pudo encontrar la partición '%s'", mounted.Name)
	}

	// Leer el superbloque
	file.Seek(partition.Part_start, 0)
	var superblock structs.SuperBloque
	if err := binary.Read(file, binary.LittleEndian, &superblock); err != nil {
		return nil, nil, fmt.Errorf("error al leer el superbloque: %v", err)
	}

	return partition, &superblock, nil
}

// Buscar archivo en el directorio raíz
func findFileInRootDirectory(file *os.File, superblock *structs.SuperBloque, fileName string) (int64, error) {
	file.Seek(superblock.S_block_start, 0)

	var rootBlock structs.BloqueCarpeta
	if err := binary.Read(file, binary.LittleEndian, &rootBlock); err != nil {
		return -1, fmt.Errorf("error al leer el directorio raíz: %v", err)
	}

	for i := 0; i < 4; i++ {
		if rootBlock.BContent[i].BInodo != -1 {
			entryName := string(rootBlock.BContent[i].BName[:])
			entryName = strings.Trim(entryName, "\x00")

			if entryName == fileName {
				return rootBlock.BContent[i].BInodo, nil
			}
		}
	}

	return -1, fmt.Errorf("archivo no encontrado")
}

// Leer contenido de archivo multi-bloque
func readFileContentMultiBlock(file *os.File, superblock *structs.SuperBloque, fileInode *structs.Inodos) (string, error) {
	var content strings.Builder

	for i := 0; i < 15; i++ {
		if fileInode.I_block[i] == -1 {
			break
		}

		blockPosition := superblock.S_block_start + (fileInode.I_block[i] * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var fileBlock structs.BloqueArchivo
		if err := binary.Read(file, binary.LittleEndian, &fileBlock); err != nil {
			return "", fmt.Errorf("error al leer el bloque %d: %v", i, err)
		}

		remainingBytes := fileInode.I_s - int64(content.Len())
		if remainingBytes <= 0 {
			break
		}

		var blockContent string
		if remainingBytes >= int64(len(fileBlock.BContent)) {
			blockContent = string(fileBlock.BContent[:])
		} else {
			blockContent = string(fileBlock.BContent[:remainingBytes])
		}

		blockContent = strings.TrimRight(blockContent, "\x00")
		content.WriteString(blockContent)

		if int64(content.Len()) >= fileInode.I_s {
			break
		}
	}

	result := content.String()
	if int64(len(result)) > fileInode.I_s {
		result = result[:fileInode.I_s]
	}

	return result, nil
}

// Escribir contenido de archivo multi-bloque
func writeFileContentMultiBlock(file *os.File, superblock *structs.SuperBloque, fileInode *structs.Inodos, newContent string, inodePosition int64) error {
	blockSize := len(structs.BloqueArchivo{}.BContent)
	contentBytes := []byte(newContent)
	blocksNeeded := (len(contentBytes) + blockSize - 1) / blockSize

	if blocksNeeded > 15 {
		return fmt.Errorf("el archivo es demasiado grande: necesita %d bloques, máximo 15", blocksNeeded)
	}

	// Actualizar el tamaño del archivo
	fileInode.I_s = int64(len(newContent))

	// Contar bloques actuales
	currentBlocks := 0
	for i := 0; i < 15; i++ {
		if fileInode.I_block[i] != -1 {
			currentBlocks++
		} else {
			break
		}
	}

	// Asignar bloques adicionales si es necesario
	if blocksNeeded > currentBlocks {
		for i := currentBlocks; i < blocksNeeded; i++ {
			newBlockIndex, err := findFreeBlock(file, superblock)
			if err != nil {
				return fmt.Errorf("no se pudo asignar bloque %d: %v", i, err)
			}
			fileInode.I_block[i] = newBlockIndex

			if err := markBlockAsUsed(file, superblock, newBlockIndex); err != nil {
				return fmt.Errorf("error al marcar bloque como usado: %v", err)
			}

		}
	}

	// Escribir el inodo actualizado
	file.Seek(inodePosition, 0)
	if err := binary.Write(file, binary.LittleEndian, fileInode); err != nil {
		return fmt.Errorf("error al escribir el inodo actualizado: %v", err)
	}

	// Escribir contenido en múltiples bloques
	for blockIndex := 0; blockIndex < blocksNeeded; blockIndex++ {
		if fileInode.I_block[blockIndex] == -1 {
			return fmt.Errorf("bloque %d no está asignado", blockIndex)
		}

		startByte := blockIndex * blockSize
		endByte := startByte + blockSize
		if endByte > len(contentBytes) {
			endByte = len(contentBytes)
		}

		blockContent := contentBytes[startByte:endByte]

		blockPosition := superblock.S_block_start + (fileInode.I_block[blockIndex] * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var fileBlock structs.BloqueArchivo
		for i := range fileBlock.BContent {
			fileBlock.BContent[i] = 0
		}

		copy(fileBlock.BContent[:], blockContent)

		if err := binary.Write(file, binary.LittleEndian, &fileBlock); err != nil {
			return fmt.Errorf("error al escribir el bloque %d: %v", blockIndex, err)
		}

	}

	return nil
}

// Buscar bloque libre
func findFreeBlock(file *os.File, superblock *structs.SuperBloque) (int64, error) {
	file.Seek(superblock.S_bm_block_start, 0)
	bitmap := make([]byte, superblock.S_blocks_count)
	if _, err := file.Read(bitmap); err != nil {
		return -1, fmt.Errorf("error al leer bitmap de bloques: %v", err)
	}

	for i := int64(0); i < superblock.S_blocks_count; i++ {
		if bitmap[i] == 0 {
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay bloques libres disponibles")
}

// Marcar bloque como usado
func markBlockAsUsed(file *os.File, superblock *structs.SuperBloque, blockIndex int64) error {
	bitmapPosition := superblock.S_bm_block_start + blockIndex
	file.Seek(bitmapPosition, 0)

	if _, err := file.Write([]byte{1}); err != nil {
		return fmt.Errorf("error al marcar bloque como usado: %v", err)
	}

	return nil
}

// Buscar inodo libre
func findFreeInode(file *os.File, superblock *structs.SuperBloque) (int64, error) {
	file.Seek(superblock.S_bm_inode_start, 0)
	bitmap := make([]byte, superblock.S_inodes_count)
	if _, err := file.Read(bitmap); err != nil {
		return -1, fmt.Errorf("error al leer bitmap de inodos: %v", err)
	}

	for i := int64(0); i < superblock.S_inodes_count; i++ {
		if bitmap[i] == 0 {
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay inodos libres disponibles")
}

// Marcar inodo como usado
func markInodeAsUsed(file *os.File, superblock *structs.SuperBloque, inodeIndex int64) error {
	bitmapPosition := superblock.S_bm_inode_start + inodeIndex
	file.Seek(bitmapPosition, 0)

	if _, err := file.Write([]byte{1}); err != nil {
		return fmt.Errorf("error al marcar inodo como usado: %v", err)
	}

	return nil
}

// Marcar inodo como libre
func markInodeAsFree(file *os.File, superblock *structs.SuperBloque, inodeIndex int64) error {
	bitmapPosition := superblock.S_bm_inode_start + inodeIndex
	file.Seek(bitmapPosition, 0)

	if _, err := file.Write([]byte{0}); err != nil {
		return fmt.Errorf("error al marcar inodo como libre: %v", err)
	}

	return nil
}

// Marcar bloque como libre
func markBlockAsFree(file *os.File, superblock *structs.SuperBloque, blockIndex int64) error {
	bitmapPosition := superblock.S_bm_block_start + blockIndex
	file.Seek(bitmapPosition, 0)

	if _, err := file.Write([]byte{0}); err != nil {
		return fmt.Errorf("error al marcar bloque como libre: %v", err)
	}

	return nil
}

// Estructura para parsear rutas
type ParsedPath struct {
	IsAbsolute  bool
	Directories []string
	FileName    string
	FullPath    string
}

// Parsear ruta del archivo con soporte para espacios en blanco
func parsePath(path string) *ParsedPath {
	// Limpiar la ruta
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	// Manejar comillas para espacios en blanco
	originalPath := path

	// Si la ruta está entre comillas, removerlas
	if (strings.HasPrefix(path, "\"") && strings.HasSuffix(path, "\"")) ||
		(strings.HasPrefix(path, "'") && strings.HasSuffix(path, "'")) {
		path = path[1 : len(path)-1]
	}

	// Verificar si es absoluta
	isAbsolute := strings.HasPrefix(path, "/")

	// Separar por "/" manteniendo espacios
	parts := strings.Split(path, "/")

	// Filtrar partes vacías pero mantener espacios
	var cleanParts []string
	for _, part := range parts {
		if part != "" {
			cleanParts = append(cleanParts, part)
		}
	}

	if len(cleanParts) == 0 {
		return &ParsedPath{
			IsAbsolute:  isAbsolute,
			Directories: []string{},
			FileName:    "",
			FullPath:    originalPath,
		}
	}

	// Último elemento es el nombre del archivo
	fileName := cleanParts[len(cleanParts)-1]
	directories := cleanParts[:len(cleanParts)-1]

	return &ParsedPath{
		IsAbsolute:  isAbsolute,
		Directories: directories,
		FileName:    fileName,
		FullPath:    originalPath,
	}
}

// Buscar inodo en un directorio (función global)
func findInodeInDirectory(file *os.File, superblock *structs.SuperBloque, dirInodeIndex int64, itemName string) (int64, error) {
	// Leer el inodo del directorio
	inodePosition := superblock.S_inode_start + (dirInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var dirInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &dirInode); err != nil {
		return -1, fmt.Errorf("error al leer inodo del directorio: %v", err)
	}

	// Verificar que es un directorio
	if dirInode.I_type != '0' {
		return -1, fmt.Errorf("no es un directorio")
	}

	// Buscar en todos los bloques del directorio
	for i := 0; i < 15; i++ {
		if dirInode.I_block[i] == -1 {
			break
		}

		blockPosition := superblock.S_block_start + (dirInode.I_block[i] * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var dirBlock structs.BloqueCarpeta
		if err := binary.Read(file, binary.LittleEndian, &dirBlock); err != nil {
			return -1, fmt.Errorf("error al leer bloque del directorio: %v", err)
		}

		// Buscar en las entradas del bloque
		for j := 0; j < 4; j++ {
			if dirBlock.BContent[j].BInodo != -1 {
				entryName := string(dirBlock.BContent[j].BName[:])
				entryName = strings.Trim(entryName, "\x00")

				// Comparación exacta incluyendo espacios
				if entryName == itemName {
					return dirBlock.BContent[j].BInodo, nil
				}
			}
		}
	}

	return -1, fmt.Errorf("elemento no encontrado")
}
