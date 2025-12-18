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

func ExecuteMkfile(path string, recursive bool, size int, contentFile string) {
	//verificar sesion activa
	if !RequireActiveSession() {
		return
	}

	//validar parametros obligatorios
	if path == "" {
		fmt.Println("Error: el parámetro -path es obligatorio para mkfile.")
		return
	}

	//validar que el tamano no sea negativo
	if size < 0 {
		fmt.Println("Error: el parámetro -size no puede ser negativo.")
		return
	}

	//obtener sesion actual
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("Error: No se pudo obtener la sesión actual.")
		return
	}

	//buscar la particion montada de la sesion
	mounted := GetMountedPartition(session.PartitionID)
	if mounted == nil {
		fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
		return
	}

	//crear el archivo
	err := createFile(mounted, path, recursive, size, contentFile, session)
	if err != nil {
		fmt.Printf("Error al crear el archivo '%s': %v\n", path, err)
		return
	}

	fmt.Printf("Archivo '%s' creado exitosamente.\n", path)
}

// crear archivo con todas las validaciones
func createFile(mounted *MountedPartition, filePath string, recursive bool, size int, contentFile string, session *Session) error {
	parsedPath := parsePath(filePath)
	if parsedPath == nil {
		return fmt.Errorf("ruta inválida: %s", filePath)
	}

	//validar que es una ruta absoluta
	if !parsedPath.IsAbsolute {
		return fmt.Errorf("la ruta debe ser absoluta: %s", filePath)
	}

	//obtener informacion del sistema de archivos
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	_, superblock, err := getPartitionAndSuperblock(file, mounted)
	if err != nil {
		return err
	}

	//verificar o crear directorios padre
	parentInode, err := ensureParentDirectories(file, superblock, parsedPath, recursive, session)
	if err != nil {
		return err
	}

	//verificar si el archivo ya existe
	exists, existingInode, err := checkFileExists(file, superblock, parsedPath)
	if err != nil {
		return fmt.Errorf("error al verificar existencia del archivo: %v", err)
	}

	if exists {
		//preguntar si se desea sobreescribir
		var response string
		fmt.Printf("El archivo '%s' ya existe. ¿Desea sobreescribirlo? (s/n): ", filePath)
		fmt.Scanln(&response)

		if strings.ToLower(response) != "s" && strings.ToLower(response) != "si" {
			return fmt.Errorf("operacion cancelada por el usuario")
		}

		//eliminar el archivo existente
		err := deleteExistingFile(file, superblock, existingInode)
		if err != nil {
			return fmt.Errorf("error al eliminar archivo existente: %v", err)
		}
	}

	//verificar permisos de escritura en el directorio padre
	if session.User != "root" {
		hasPermission, err := checkWritePermission(file, superblock, parentInode, session)
		if err != nil {
			return fmt.Errorf("error al verificar permisos: %v", err)
		}
		if !hasPermission {
			return fmt.Errorf("sin permisos de escritura en el directorio padre")
		}
	}

	//generar contenido del archivo
	content, err := generateFileContent(size, contentFile)
	if err != nil {
		return fmt.Errorf("error al generar contenido: %v", err)
	}

	//crear el archivo
	err = createNewFile(file, superblock, parsedPath.FileName, content, session, parentInode)
	if err != nil {
		return fmt.Errorf("error al crear archivo: %v", err)
	}

	return nil
}

// verificar si el archivo ya existe
func checkFileExists(file *os.File, superblock *structs.SuperBloque, parsedPath *ParsedPath) (bool, int64, error) {
	currentInodeIndex := int64(0)

	for _, dirName := range parsedPath.Directories {
		nextInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, dirName)
		if err != nil {
			return false, -1, nil
		}
		currentInodeIndex = nextInode
	}

	//buscar el archivo en el directorio padre
	fileInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, parsedPath.FileName)
	if err != nil {
		return false, -1, nil
	}

	return true, fileInode, nil
}

// asegurar que los directorios padre existen
func ensureParentDirectories(file *os.File, superblock *structs.SuperBloque, parsedPath *ParsedPath, recursive bool, session *Session) (int64, error) {
	currentInodeIndex := int64(0)

	for _, dirName := range parsedPath.Directories {
		nextInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, dirName)
		if err != nil {
			if !recursive {
				return -1, fmt.Errorf("el directorio '%s' no existe. Use -r para crear directorios padre", dirName)
			}

			//crear el directorio
			newDirInode, err := createDirectory(file, superblock, currentInodeIndex, dirName, session)
			if err != nil {
				return -1, fmt.Errorf("error al crear directorio '%s': %v", dirName, err)
			}

			currentInodeIndex = newDirInode
		} else {
			currentInodeIndex = nextInode
		}
	}

	return currentInodeIndex, nil
}

// crear un nuevo directorio
func createDirectory(file *os.File, superblock *structs.SuperBloque, parentInodeIndex int64, dirName string, session *Session) (int64, error) {
	newInodeIndex, err := findFreeInode(file, superblock)
	if err != nil {
		return -1, fmt.Errorf("no hay inodos libres: %v", err)
	}

	//buscar bloque libre
	newBlockIndex, err := findFreeBlock(file, superblock)
	if err != nil {
		return -1, fmt.Errorf("no hay bloques libres: %v", err)
	}

	//crear el inodo del directorio
	var newDirInode structs.Inodos
	newDirInode.I_uid = 1
	newDirInode.I_gid = 1
	newDirInode.I_s = int64(binary.Size(structs.BloqueCarpeta{}))
	newDirInode.I_type = '0'
	newDirInode.I_perm = [3]byte{6, 6, 4}

	currentTime := time.Now().Unix()
	newDirInode.I_atime = currentTime
	newDirInode.I_ctime = currentTime
	newDirInode.I_mtime = currentTime

	//configurar usuario y grupo segun la sesion
	if session.User != "root" {
		userInfo, err := getUserInfo(file, superblock, session.User)
		if err == nil {
			newDirInode.I_uid = userInfo.UID
			newDirInode.I_gid = userInfo.GID
		}
	}

	//inicializar bloques
	for i := range newDirInode.I_block {
		newDirInode.I_block[i] = -1
	}
	newDirInode.I_block[0] = newBlockIndex

	//escribir el inodo
	inodePosition := superblock.S_inode_start + (newInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &newDirInode); err != nil {
		return -1, fmt.Errorf("error al escribir inodo del directorio: %v", err)
	}

	//crear el bloque del directorio con entradas punto y punto punto
	var dirBlock structs.BloqueCarpeta

	//inicializar todas las entradas como vacias
	for i := range dirBlock.BContent {
		dirBlock.BContent[i].BInodo = -1
		for j := range dirBlock.BContent[i].BName {
			dirBlock.BContent[i].BName[j] = 0
		}
	}

	//entrada punto
	copy(dirBlock.BContent[0].BName[:], []byte("."))
	dirBlock.BContent[0].BInodo = newInodeIndex

	//entrada punto punto
	copy(dirBlock.BContent[1].BName[:], []byte(".."))
	dirBlock.BContent[1].BInodo = parentInodeIndex

	//escribir el bloque del directorio
	blockPosition := superblock.S_block_start + (newBlockIndex * superblock.S_block_s)
	file.Seek(blockPosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &dirBlock); err != nil {
		return -1, fmt.Errorf("error al escribir bloque del directorio: %v", err)
	}

	//marcar inodo y bloque como usados
	if err := markInodeAsUsed(file, superblock, newInodeIndex); err != nil {
		return -1, fmt.Errorf("error al marcar inodo como usado: %v", err)
	}
	if err := markBlockAsUsed(file, superblock, newBlockIndex); err != nil {
		return -1, fmt.Errorf("error al marcar bloque como usado: %v", err)
	}

	//agregar entrada al directorio padre
	err = addEntryToDirectory(file, superblock, parentInodeIndex, dirName, newInodeIndex)
	if err != nil {
		return -1, fmt.Errorf("error al agregar entrada al directorio padre: %v", err)
	}

	fmt.Printf("Directorio '%s' creado\n", dirName)
	return newInodeIndex, nil
}

// estructura para informacion de usuario
type UserInfo struct {
	UID       int64
	GID       int64
	Username  string
	GroupName string
}

// obtener informacion del usuario
func getUserInfo(file *os.File, superblock *structs.SuperBloque, username string) (*UserInfo, error) {
	usersContent, err := readFileByName(file, superblock, "users.txt")
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

				//buscar el gid del grupo
				gid, err := getGroupGID(usersContent, groupName)
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

// obtener gid de un grupo
func getGroupGID(usersContent, groupName string) (int64, error) {
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

// verificar permisos de escritura
func checkWritePermission(file *os.File, superblock *structs.SuperBloque, dirInodeIndex int64, session *Session) (bool, error) {
	//leer el inodo del directorio
	inodePosition := superblock.S_inode_start + (dirInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var dirInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &dirInode); err != nil {
		return false, fmt.Errorf("error al leer inodo del directorio: %v", err)
	}

	//si es root siempre tiene permisos
	if session.User == "root" {
		return true, nil
	}

	//obtener informacion del usuario actual
	userInfo, err := getUserInfo(file, superblock, session.User)
	if err != nil {
		return false, fmt.Errorf("error al obtener informacion del usuario: %v", err)
	}

	//determinar categoria del usuario
	var permissionIndex int
	if userInfo.UID == dirInode.I_uid {
		permissionIndex = 0
	} else if userInfo.GID == dirInode.I_gid {
		permissionIndex = 1
	} else {
		permissionIndex = 2
	}

	//verificar permiso de escritura
	permission := dirInode.I_perm[permissionIndex]
	hasWritePermission := (permission & 2) != 0

	return hasWritePermission, nil
}

// generar contenido del archivo
func generateFileContent(size int, contentFile string) (string, error) {
	if contentFile != "" {
		content, err := os.ReadFile(contentFile)
		if err != nil {
			return "", fmt.Errorf("no se pudo leer el archivo '%s': %v", contentFile, err)
		}
		return string(content), nil
	}
	if size == 0 {
		return "", nil
	}

	//generar contenido con numeros 0 a 9
	var content strings.Builder
	for i := 0; i < size; i++ {
		digit := i % 10
		content.WriteString(strconv.Itoa(digit))
	}

	return content.String(), nil
}

// crear nuevo archivo
func createNewFile(file *os.File, superblock *structs.SuperBloque, fileName, content string, session *Session, parentInodeIndex int64) error {
	newInodeIndex, err := findFreeInode(file, superblock)
	if err != nil {
		return fmt.Errorf("no hay inodos libres: %v", err)
	}

	//crear el inodo del archivo
	var newFileInode structs.Inodos
	newFileInode.I_uid = 1
	newFileInode.I_gid = 1
	newFileInode.I_s = int64(len(content))
	newFileInode.I_type = '1'
	newFileInode.I_perm = [3]byte{6, 6, 4}

	currentTime := time.Now().Unix()
	newFileInode.I_atime = currentTime
	newFileInode.I_ctime = currentTime
	newFileInode.I_mtime = currentTime

	//configurar usuario y grupo segun la sesion
	if session.User != "root" {
		userInfo, err := getUserInfo(file, superblock, session.User)
		if err == nil {
			newFileInode.I_uid = userInfo.UID
			newFileInode.I_gid = userInfo.GID
		}
	}

	//inicializar bloques
	for i := range newFileInode.I_block {
		newFileInode.I_block[i] = -1
	}

	//escribir el inodo
	inodePosition := superblock.S_inode_start + (newInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &newFileInode); err != nil {
		return fmt.Errorf("error al escribir inodo del archivo: %v", err)
	}

	//escribir contenido multi bloque
	err = writeFileContentMultiBlock(file, superblock, &newFileInode, content, inodePosition)
	if err != nil {
		return fmt.Errorf("error al escribir contenido del archivo: %v", err)
	}

	//marcar inodo como usado
	if err := markInodeAsUsed(file, superblock, newInodeIndex); err != nil {
		return fmt.Errorf("error al marcar inodo como usado: %v", err)
	}

	//agregar entrada al directorio padre
	err = addEntryToDirectory(file, superblock, parentInodeIndex, fileName, newInodeIndex)
	if err != nil {
		return fmt.Errorf("error al agregar entrada al directorio padre: %v", err)
	}

	return nil
}

// agregar entrada a un directorio
func addEntryToDirectory(file *os.File, superblock *structs.SuperBloque, dirInodeIndex int64, itemName string, itemInodeIndex int64) error {
	if len(itemName) > 12 {
		return fmt.Errorf("nombre demasiado largo: '%s' (maximo 12 caracteres)", itemName)
	}

	//leer el inodo del directorio
	inodePosition := superblock.S_inode_start + (dirInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var dirInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &dirInode); err != nil {
		return fmt.Errorf("error al leer inodo del directorio: %v", err)
	}

	//buscar un espacio libre en los bloques existentes
	for i := 0; i < 15; i++ {
		if dirInode.I_block[i] == -1 {
			newBlockIndex, err := findFreeBlock(file, superblock)
			if err != nil {
				return fmt.Errorf("no hay bloques libres: %v", err)
			}

			dirInode.I_block[i] = newBlockIndex

			var dirBlock structs.BloqueCarpeta
			for j := range dirBlock.BContent {
				dirBlock.BContent[j].BInodo = -1
				for k := range dirBlock.BContent[j].BName {
					dirBlock.BContent[j].BName[k] = 0
				}
			}

			//agregar la entrada preservando espacios
			nameBytes := []byte(itemName)
			if len(nameBytes) > 12 {
				nameBytes = nameBytes[:12]
			}
			copy(dirBlock.BContent[0].BName[:], nameBytes)
			dirBlock.BContent[0].BInodo = itemInodeIndex

			blockPosition := superblock.S_block_start + (newBlockIndex * superblock.S_block_s)
			file.Seek(blockPosition, 0)
			if err := binary.Write(file, binary.LittleEndian, &dirBlock); err != nil {
				return fmt.Errorf("error al escribir nuevo bloque del directorio: %v", err)
			}

			file.Seek(inodePosition, 0)
			if err := binary.Write(file, binary.LittleEndian, &dirInode); err != nil {
				return fmt.Errorf("error al actualizar inodo del directorio: %v", err)
			}

			if err := markBlockAsUsed(file, superblock, newBlockIndex); err != nil {
				return fmt.Errorf("error al marcar bloque como usado: %v", err)
			}

			return nil
		}

		blockPosition := superblock.S_block_start + (dirInode.I_block[i] * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var dirBlock structs.BloqueCarpeta
		if err := binary.Read(file, binary.LittleEndian, &dirBlock); err != nil {
			return fmt.Errorf("error al leer bloque del directorio: %v", err)
		}

		//buscar entrada libre en este bloque
		for j := 0; j < 4; j++ {
			if dirBlock.BContent[j].BInodo == -1 {
				nameBytes := []byte(itemName)
				if len(nameBytes) > 12 {
					nameBytes = nameBytes[:12]
				}
				copy(dirBlock.BContent[j].BName[:], nameBytes)
				dirBlock.BContent[j].BInodo = itemInodeIndex

				file.Seek(blockPosition, 0)
				if err := binary.Write(file, binary.LittleEndian, &dirBlock); err != nil {
					return fmt.Errorf("error al escribir bloque del directorio: %v", err)
				}

				return nil
			}
		}
	}

	return fmt.Errorf("directorio lleno no se puede agregar mas entradas")
}

// eliminar archivo existente
func deleteExistingFile(file *os.File, superblock *structs.SuperBloque, fileInodeIndex int64) error {
	inodePosition := superblock.S_inode_start + (fileInodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var fileInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &fileInode); err != nil {
		return fmt.Errorf("error al leer inodo del archivo: %v", err)
	}

	//liberar todos los bloques del archivo
	for i := 0; i < 15; i++ {
		if fileInode.I_block[i] != -1 {
			if err := markBlockAsFree(file, superblock, fileInode.I_block[i]); err != nil {
				return fmt.Errorf("error al liberar bloque %d: %v", fileInode.I_block[i], err)
			}
		}
	}

	//marcar inodo como libre
	if err := markInodeAsFree(file, superblock, fileInodeIndex); err != nil {
		return fmt.Errorf("error al liberar inodo: %v", err)
	}

	return nil
}

// leer archivo por nombre
func readFileByName(file *os.File, superblock *structs.SuperBloque, fileName string) (string, error) {
	inodeIndex, err := findFileInRootDirectory(file, superblock, fileName)
	if err != nil {
		return "", fmt.Errorf("archivo '%s' no encontrado: %v", fileName, err)
	}

	//leer el inodo del archivo
	inodePosition := superblock.S_inode_start + (inodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var fileInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &fileInode); err != nil {
		return "", fmt.Errorf("error al leer inodo de '%s': %v", fileName, err)
	}

	//leer el contenido completo
	content, err := readFileContentMultiBlock(file, superblock, &fileInode)
	if err != nil {
		return "", fmt.Errorf("error al leer contenido de '%s': %v", fileName, err)
	}

	return content, nil
}
