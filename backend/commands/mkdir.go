package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func ExecuteMkdir(path string, parents bool) {
	// Verificar sesión activa
	if !RequireActiveSession() {
		return
	}

	// Validar parámetros obligatorios
	if path == "" {
		fmt.Println("Error: el parámetro -path es obligatorio para mkdir.")
		return
	}

	// Obtener sesión actual
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("Error: No se pudo obtener la sesión actual.")
		return
	}

	// Buscar la partición montada de la sesión
	mounted := GetMountedPartition(session.PartitionID)
	if mounted == nil {
		fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
		return
	}

	// Crear el directorio
	err := createDirectoryPath(mounted, path, parents, session)
	if err != nil {
		fmt.Printf("Error al crear el directorio '%s': %v\n", path, err)
		return
	}

	fmt.Printf("Directorio '%s' creado exitosamente.\n", path)
}

// Crear directorio con todas las validaciones
func createDirectoryPath(mounted *MountedPartition, dirPath string, parents bool, session *Session) error {
	// Parsear la ruta del directorio
	parsedPath := parsePath(dirPath)
	if parsedPath == nil {
		return fmt.Errorf("ruta inválida: %s", dirPath)
	}

	// Validar que es una ruta absoluta
	if !parsedPath.IsAbsolute {
		return fmt.Errorf("la ruta debe ser absoluta: %s", dirPath)
	}

	// Obtener información del sistema de archivos
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	_, superblock, err := getPartitionAndSuperblock(file, mounted)
	if err != nil {
		return err
	}

	// Verificar si el directorio ya existe
	exists, err := checkDirectoryExists(file, superblock, parsedPath)
	if err != nil {
		return fmt.Errorf("error al verificar existencia del directorio: %v", err)
	}

	if exists {
		// El directorio ya existe, no hacer nada (comportamiento estándar de mkdir)
		fmt.Printf("El directorio '%s' ya existe.\n", dirPath)
		return nil
	}

	// Asegurar que los directorios padre existen
	parentInode, err := ensureParentDirectoriesForMkdir(file, superblock, parsedPath, parents, session)
	if err != nil {
		return err
	}

	// Verificar permisos de escritura en el directorio padre
	if session.User != "root" {
		hasPermission, err := checkWritePermissionForMkdir(file, superblock, parentInode, session)
		if err != nil {
			return fmt.Errorf("error al verificar permisos: %v", err)
		}
		if !hasPermission {
			return fmt.Errorf("sin permisos de escritura en el directorio padre")
		}
	}

	// Crear el directorio final
	_, err = createDirectoryForMkdir(file, superblock, parentInode, parsedPath.FileName, session)
	if err != nil {
		return fmt.Errorf("error al crear directorio: %v", err)
	}

	return nil
}

// Verificar si el directorio ya existe
func checkDirectoryExists(file *os.File, superblock *structs.SuperBloque, parsedPath *ParsedPath) (bool, error) {
	// Navegar hasta el directorio completo (incluyendo el directorio final)
	currentInodeIndex := int64(0) // Empezar desde el directorio raíz

	// Navegar por todos los directorios incluyendo el final
	allDirs := append(parsedPath.Directories, parsedPath.FileName)

	for _, dirName := range allDirs {
		nextInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, dirName)
		if err != nil {
			// El directorio no existe
			return false, nil
		}
		currentInodeIndex = nextInode
	}

	return true, nil
}

// Asegurar que los directorios padre existen (específico para mkdir)
func ensureParentDirectoriesForMkdir(file *os.File, superblock *structs.SuperBloque, parsedPath *ParsedPath, parents bool, session *Session) (int64, error) {
	currentInodeIndex := int64(0) // Empezar desde el directorio raíz

	for _, dirName := range parsedPath.Directories {
		nextInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, dirName)
		if err != nil {
			// El directorio no existe
			if !parents {
				return -1, fmt.Errorf("el directorio padre '%s' no existe. Use -p para crear directorios padre", dirName)
			}

			// Crear el directorio padre
			newDirInode, err := createDirectoryForMkdir(file, superblock, currentInodeIndex, dirName, session)
			if err != nil {
				return -1, fmt.Errorf("error al crear directorio padre '%s': %v", dirName, err)
			}

			currentInodeIndex = newDirInode
		} else {
			currentInodeIndex = nextInode
		}
	}

	return currentInodeIndex, nil
}

// Crear un nuevo directorio (específico para mkdir)
func createDirectoryForMkdir(file *os.File, superblock *structs.SuperBloque, parentInodeIndex int64, dirName string, session *Session) (int64, error) {
	// Validar longitud del nombre
	if len(dirName) > 12 {
		return -1, fmt.Errorf("nombre del directorio demasiado largo: '%s' (máximo 12 caracteres)", dirName)
	}

	// Buscar inodo libre
	newInodeIndex, err := findFreeInode(file, superblock)
	if err != nil {
		return -1, fmt.Errorf("no hay inodos libres: %v", err)
	}

	// Buscar bloque libre
	newBlockIndex, err := findFreeBlock(file, superblock)
	if err != nil {
		return -1, fmt.Errorf("no hay bloques libres: %v", err)
	}

	// Crear el inodo del directorio
	var newDirInode structs.Inodos
	newDirInode.I_uid = 1 // Por defecto, root
	newDirInode.I_gid = 1 // Por defecto, root group
	newDirInode.I_s = int64(binary.Size(structs.BloqueCarpeta{}))
	newDirInode.I_type = '0'              // Directorio
	newDirInode.I_perm = [3]byte{6, 6, 4} // 664 por defecto

	currentTime := time.Now().Unix()
	newDirInode.I_atime = currentTime // Tiempo de acceso
	newDirInode.I_ctime = currentTime // Tiempo de creación
	newDirInode.I_mtime = currentTime // Tiempo de modificación

	// Configurar usuario y grupo según la sesión
	if session.User != "root" {
		userInfo, err := getUserInfoForMkdir(file, superblock, session.User)
		if err == nil {
			newDirInode.I_uid = userInfo.UID
			newDirInode.I_gid = userInfo.GID
		}
	}

	// Inicializar bloques
	for i := range newDirInode.I_block {
		newDirInode.I_block[i] = -1
	}
	newDirInode.I_block[0] = newBlockIndex

	// Escribir el inodo
	inodePosition := superblock.S_inode_start + (newInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &newDirInode); err != nil {
		return -1, fmt.Errorf("error al escribir inodo del directorio: %v", err)
	}

	// Crear el bloque del directorio con entradas . y ..
	var dirBlock structs.BloqueCarpeta

	// Inicializar todas las entradas como vacías
	for i := range dirBlock.BContent {
		dirBlock.BContent[i].BInodo = -1
		for j := range dirBlock.BContent[i].BName {
			dirBlock.BContent[i].BName[j] = 0
		}
	}

	// Entrada "." (directorio actual)
	copy(dirBlock.BContent[0].BName[:], []byte("."))
	dirBlock.BContent[0].BInodo = newInodeIndex

	// Entrada ".." (directorio padre)
	copy(dirBlock.BContent[1].BName[:], []byte(".."))
	dirBlock.BContent[1].BInodo = parentInodeIndex

	// Escribir el bloque del directorio
	blockPosition := superblock.S_block_start + (newBlockIndex * superblock.S_block_s)
	file.Seek(blockPosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &dirBlock); err != nil {
		return -1, fmt.Errorf("error al escribir bloque del directorio: %v", err)
	}

	// Marcar inodo y bloque como usados
	if err := markInodeAsUsed(file, superblock, newInodeIndex); err != nil {
		return -1, fmt.Errorf("error al marcar inodo como usado: %v", err)
	}
	if err := markBlockAsUsed(file, superblock, newBlockIndex); err != nil {
		return -1, fmt.Errorf("error al marcar bloque como usado: %v", err)
	}

	// Agregar entrada al directorio padre
	err = addEntryToDirectoryForMkdir(file, superblock, parentInodeIndex, dirName, newInodeIndex)
	if err != nil {
		return -1, fmt.Errorf("error al agregar entrada al directorio padre: %v", err)
	}

	fmt.Printf("✅ Directorio '%s' creado\n", dirName)
	return newInodeIndex, nil
}

// Verificar permisos de escritura (específico para mkdir)
func checkWritePermissionForMkdir(file *os.File, superblock *structs.SuperBloque, dirInodeIndex int64, session *Session) (bool, error) {
	// Leer el inodo del directorio
	inodePosition := superblock.S_inode_start + (dirInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var dirInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &dirInode); err != nil {
		return false, fmt.Errorf("error al leer inodo del directorio: %v", err)
	}

	// Si es root, siempre tiene permisos
	if session.User == "root" {
		return true, nil
	}

	// Obtener información del usuario actual
	userInfo, err := getUserInfoForMkdir(file, superblock, session.User)
	if err != nil {
		return false, fmt.Errorf("error al obtener información del usuario: %v", err)
	}

	// Determinar categoría del usuario (User, Group, Other)
	var permissionIndex int
	if userInfo.UID == dirInode.I_uid {
		// Es el propietario
		permissionIndex = 0 // User
	} else if userInfo.GID == dirInode.I_gid {
		// Pertenece al mismo grupo
		permissionIndex = 1 // Group
	} else {
		// Otro usuario
		permissionIndex = 2 // Other
	}

	// Verificar permiso de escritura (bit 1)
	permission := dirInode.I_perm[permissionIndex]
	hasWritePermission := (permission & 2) != 0 // Bit de escritura

	return hasWritePermission, nil
}

// Obtener información del usuario (específico para mkdir)
func getUserInfoForMkdir(file *os.File, superblock *structs.SuperBloque, username string) (*UserInfo, error) {
	// Leer el archivo users.txt
	usersContent, err := readFileByNameForMkdir(file, superblock, "users.txt")
	if err != nil {
		return nil, fmt.Errorf("error al leer users.txt: %v", err)
	}

	lines := strings.Split(usersContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 5 {
			uidStr := strings.TrimSpace(parts[0])
			tipo := strings.TrimSpace(parts[1])
			groupName := strings.TrimSpace(parts[2])
			user := strings.TrimSpace(parts[3])

			if tipo == "U" && user == username {
				uid, err := strconv.ParseInt(uidStr, 10, 64)
				if err != nil {
					continue
				}

				// Buscar el GID del grupo
				gid, err := getGroupGIDForMkdir(usersContent, groupName)
				if err != nil {
					continue
				}

				return &UserInfo{
					UID:       uid,
					GID:       gid,
					Username:  username,
					GroupName: groupName,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("usuario no encontrado")
}

// Obtener GID de un grupo (específico para mkdir)
func getGroupGIDForMkdir(usersContent, groupName string) (int64, error) {
	lines := strings.Split(usersContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			gidStr := strings.TrimSpace(parts[0])
			tipo := strings.TrimSpace(parts[1])
			group := strings.TrimSpace(parts[2])

			if tipo == "G" && group == groupName {
				gid, err := strconv.ParseInt(gidStr, 10, 64)
				if err != nil {
					continue
				}
				return gid, nil
			}
		}
	}

	return -1, fmt.Errorf("grupo no encontrado")
}

// Agregar entrada a un directorio (específico para mkdir)
func addEntryToDirectoryForMkdir(file *os.File, superblock *structs.SuperBloque, dirInodeIndex int64, itemName string, itemInodeIndex int64) error {
	// Validar longitud del nombre
	if len(itemName) > 12 {
		return fmt.Errorf("nombre demasiado largo: '%s' (máximo 12 caracteres)", itemName)
	}

	// Leer el inodo del directorio
	inodePosition := superblock.S_inode_start + (dirInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var dirInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &dirInode); err != nil {
		return fmt.Errorf("error al leer inodo del directorio: %v", err)
	}

	// Buscar un espacio libre en los bloques existentes
	for i := 0; i < 15; i++ {
		if dirInode.I_block[i] == -1 {
			// Necesitamos un nuevo bloque
			newBlockIndex, err := findFreeBlock(file, superblock)
			if err != nil {
				return fmt.Errorf("no hay bloques libres: %v", err)
			}

			dirInode.I_block[i] = newBlockIndex

			// Crear nuevo bloque vacío
			var dirBlock structs.BloqueCarpeta
			for j := range dirBlock.BContent {
				dirBlock.BContent[j].BInodo = -1
				for k := range dirBlock.BContent[j].BName {
					dirBlock.BContent[j].BName[k] = 0
				}
			}

			// Agregar la entrada preservando espacios
			nameBytes := []byte(itemName)
			if len(nameBytes) > 12 {
				nameBytes = nameBytes[:12]
			}
			copy(dirBlock.BContent[0].BName[:], nameBytes)
			dirBlock.BContent[0].BInodo = itemInodeIndex

			// Escribir el bloque
			blockPosition := superblock.S_block_start + (newBlockIndex * superblock.S_block_s)
			file.Seek(blockPosition, 0)
			if err := binary.Write(file, binary.LittleEndian, &dirBlock); err != nil {
				return fmt.Errorf("error al escribir nuevo bloque del directorio: %v", err)
			}

			// Actualizar el inodo del directorio
			file.Seek(inodePosition, 0)
			if err := binary.Write(file, binary.LittleEndian, &dirInode); err != nil {
				return fmt.Errorf("error al actualizar inodo del directorio: %v", err)
			}

			// Marcar bloque como usado
			if err := markBlockAsUsed(file, superblock, newBlockIndex); err != nil {
				return fmt.Errorf("error al marcar bloque como usado: %v", err)
			}

			return nil
		}

		// Verificar bloque existente
		blockPosition := superblock.S_block_start + (dirInode.I_block[i] * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var dirBlock structs.BloqueCarpeta
		if err := binary.Read(file, binary.LittleEndian, &dirBlock); err != nil {
			return fmt.Errorf("error al leer bloque del directorio: %v", err)
		}

		// Buscar entrada libre en este bloque
		for j := 0; j < 4; j++ {
			if dirBlock.BContent[j].BInodo == -1 {
				// Encontramos espacio libre, preservar espacios
				nameBytes := []byte(itemName)
				if len(nameBytes) > 12 {
					nameBytes = nameBytes[:12]
				}
				copy(dirBlock.BContent[j].BName[:], nameBytes)
				dirBlock.BContent[j].BInodo = itemInodeIndex

				// Escribir el bloque actualizado
				file.Seek(blockPosition, 0)
				if err := binary.Write(file, binary.LittleEndian, &dirBlock); err != nil {
					return fmt.Errorf("error al escribir bloque del directorio: %v", err)
				}

				return nil
			}
		}
	}

	return fmt.Errorf("directorio lleno, no se puede agregar más entradas")
}

// Leer archivo por nombre (específico para mkdir)
func readFileByNameForMkdir(file *os.File, superblock *structs.SuperBloque, fileName string) (string, error) {
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
		return "", fmt.Errorf("error al leer inodo de '%s': %v", fileName, err)
	}

	// Leer el contenido completo
	content, err := readFileContentMultiBlock(file, superblock, &fileInode)
	if err != nil {
		return "", fmt.Errorf("error al leer contenido de '%s': %v", fileName, err)
	}

	return content, nil
}
