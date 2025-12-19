package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// estructura para manejar particiones montadas en memoria
type MountedPartition struct {
	ID   string
	Path string
	Name string
	Size int64
}

// variable global para almacenar particiones montadas simulando ram
var mountedPartitions []MountedPartition
var diskCounters = make(map[string]int)

func ExecuteMount(diskName string, name string) {
	if name == "" {
		fmt.Println("Error: el parámetro -name es obligatorio para mount.")
		return
	}
	//asegurar que el nombre tiene extension .mia
	if !strings.HasSuffix(strings.ToLower(diskName), ".mia") {
		diskName += ".mia"
	}

	//buscar el disco dentro del directorio de discos
	fullPath := filepath.Join(DisksDirectory, diskName)

	//verificar que el archivo existe
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		fmt.Printf("Error: El disco '%s' no existe en el directorio de discos.\n", diskName)
		return
	}

	//abrir el archivo del disco en modo lectura escritura
	file, err := os.OpenFile(fullPath, os.O_RDWR, 0644)
	if err != nil {
		fmt.Printf("Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	//leer el mbr
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al leer el MBR: %v\n", err)
		return
	}

	//buscar la particion por nombre
	var foundPartition *structs.Partition
	var partitionIndex int = -1

	for i, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			partitionNameBytes := partition.Part_name[:]
			nullIndex := len(partitionNameBytes)
			for j, b := range partitionNameBytes {
				if b == 0 {
					nullIndex = j
					break
				}
			}
			partitionName := strings.TrimSpace(string(partitionNameBytes[:nullIndex]))

			if strings.EqualFold(partitionName, name) || partitionName == name {
				if partition.Part_type == 'P' {
					partitionCopy := partition
					foundPartition = &partitionCopy
					partitionIndex = i
					break
				} else {
					fmt.Printf("Error: Solo se pueden montar particiones primarias. La partición '%s' es de tipo '%c'.\n", name, partition.Part_type)
					return
				}
			}
		}
	}

	if foundPartition == nil {
		fmt.Printf("Error: No se encontró la partición '%s' en el disco '%s'.\n", name, diskName)
		return
	}

	//verificar si la particion ya esta montada
	for _, mounted := range mountedPartitions {
		if mounted.Path == fullPath && mounted.Name == name {
			fmt.Printf("Error: La partición '%s' del disco '%s' ya está montada con ID '%s'.\n", name, diskName, mounted.ID)
			return
		}
	}

	//generar el id y correlativo
	// generar ID usando el nombre del disco (sin ruta)
	id := generatePartitionID(diskName)
	correlativo := generateCorrelativo()

	//actualizar la particion en el mbr del disco
	mbr.Mbr_partitions[partitionIndex].Part_correlative = int64(correlativo)
	copy(mbr.Mbr_partitions[partitionIndex].Part_id[:], []byte(id)[:4])

	//escribir el mbr actualizado de vuelta al disco
	file.Seek(0, 0)
	if err := binary.Write(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al actualizar el MBR: %v\n", err)
		return
	}

	//crear la entrada de particion montada
	mountedPartition := MountedPartition{
		ID:   id,
		Path: fullPath,
		Name: name,
		Size: foundPartition.Part_s,
	}

	//agregar a la lista de particiones montadas
	mountedPartitions = append(mountedPartitions, mountedPartition)

	//mostrar mensaje de exito
	fmt.Printf("Partición '%s' montada exitosamente.\n", name)
	fmt.Printf("   Disco: %s\n", diskName)
	fmt.Printf("   ID asignado: %s\n", id)
	fmt.Printf("   Correlativo: %d\n", correlativo)
	fmt.Printf("   Tamaño: %d bytes\n", foundPartition.Part_s)
}

// generar correlativo secuencial
func generateCorrelativo() int {
	return len(mountedPartitions) + 1
}

func generatePartitionID(diskPath string) string {
	//ultimos dos digitos del carnet
	carnetSuffix := "27"

	//verificar si es el mismo disco o uno nuevo
	partitionNumber, exists := diskCounters[diskPath]
	if !exists {

		partitionNumber = 1
		diskCounters[diskPath] = partitionNumber
	} else {

		partitionNumber++
		diskCounters[diskPath] = partitionNumber
	}

	//determinar la letra segun el numero de discos montados
	currentLetter := getLetter()

	//formato ultimos 2 digitos numero de particion letra
	return fmt.Sprintf("%s%d%c", carnetSuffix, partitionNumber, currentLetter)
}

func getLetter() byte {

	uniqueDisks := len(diskCounters)

	if uniqueDisks == 1 {
		return 'A'
	}

	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if uniqueDisks-1 < len(letters) {
		return letters[uniqueDisks-1]
	}

	return 'Z'
}

func ExecuteMounted() {
	if len(mountedPartitions) == 0 {
		fmt.Println("No hay particiones montadas.")
		return
	}

	fmt.Println("Particiones montadas:")
	for _, mounted := range mountedPartitions {
		fmt.Printf("ID: %s\n", mounted.ID)
		fmt.Printf("Nombre: %s\n", mounted.Name)
		fmt.Printf("Ruta: %s\n", mounted.Path)
		fmt.Println("-------------------------")
	}
}

// funcion para obtener una particion montada por id
func GetMountedPartition(id string) *MountedPartition {
	for i, mounted := range mountedPartitions {
		if mounted.ID == id {
			return &mountedPartitions[i]
		}
	}
	return nil
}
