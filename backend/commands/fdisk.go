package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func ExecuteFdisk(size int64, unit string, diskName string, tipo string, fit string, name string) {
	//validar parametros obligatorios
	if size <= 0 {
		fmt.Printf("Error: El parámetro -size debe ser positivo y mayor a cero.\n")
		return
	}

	if diskName == "" {
		fmt.Printf("Error: El parámetro -diskName es obligatorio.\n")
		return
	}

	if name == "" {
		fmt.Printf("Error: El parámetro -name es obligatorio.\n")
		return
	}

	//manejar parametros opcionales con valores por defecto
	if tipo == "" {
		tipo = "P"
	}

	if fit == "" {
		fit = "WF"
	}

	if unit == "" {
		unit = "K"
	}

	//validar unidades
	unit = strings.ToUpper(unit)
	if unit != "B" && unit != "K" && unit != "M" {
		fmt.Printf("Error: Unidad '%s' no válida. Use 'B', 'K' o 'M'.\n", unit)
		return
	}

	//validar tipos
	tipo = strings.ToUpper(tipo)
	if tipo != "P" && tipo != "E" && tipo != "L" {
		fmt.Printf("Error: Tipo de partición '%s' no válido. Use 'P', 'E' o 'L'.\n", tipo)
		return
	}

	//validar fit
	fit = strings.ToUpper(fit)
	if fit != "BF" && fit != "FF" && fit != "WF" {
		fmt.Printf("Error: Ajuste '%s' no válido. Use 'BF', 'FF' o 'WF'.\n", fit)
		return
	}

	if !strings.HasSuffix(strings.ToLower(diskName), ".mia") {
		diskName += ".mia"
	}

	fullPath := filepath.Join(DisksDirectory, diskName)

	//verificar que el archivo existe
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		fmt.Printf("Error: El archivo '%s' no existe.\n", diskName)
		return
	}

	file, err := os.OpenFile(fullPath, os.O_RDWR, 0644)
	if err != nil {
		fmt.Printf("Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al leer el MBR: %v\n", err)
		return
	}

	//convertir tamaño segun la unidad
	sizeInBytes := convertSize(size, unit)

	if err := validatePartitionName(name, &mbr); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	//validar restricciones de particiones
	if err := validatePartitionConstraints(tipo, &mbr); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	//validar espacio disponible
	if err := validateAvailableSpace(sizeInBytes, &mbr, tipo); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	//buscar un slot libre en las particiones
	var partitionIndex int = -1
	var startPosition int64

	switch tipo {
	case "P", "E":
		partitionIndex = findFreePartitionSlot(&mbr)
		if partitionIndex == -1 {
			fmt.Printf("Error: No hay slots disponibles para particiones primarias/extendidas.\n")
			return
		}
		startPosition = calculateStartPosition(&mbr, fit, sizeInBytes)
	case "L":

		_, err = handleLogicalPartition(&mbr, sizeInBytes, fit, name, file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		fmt.Printf("Partición lógica '%s' creada exitosamente en '%s'.\n", name, diskName)
		return
	}

	//crear la nueva partición
	newPartition := structs.NewPartition(
		'1',
		tipo[0],
		fit[0],
		startPosition,
		sizeInBytes,
		[16]byte{},
	)

	//copiar el nombre de la particion
	copy(newPartition.Part_name[:], []byte(name))

	//asignar la partición al MBR
	mbr.Mbr_partitions[partitionIndex] = newPartition

	//escribir el MBR actualizado al inicio del archivo
	file.Seek(0, 0)
	if err := binary.Write(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al escribir el MBR: %v\n", err)
		return
	}

	//crear EBR si es particion extendida
	if tipo == "E" {
		ebr := structs.EBR{
			PartMount: 0,
			PartFit:   fit[0],
			PartStart: startPosition,
			PartS:     sizeInBytes,
			PartNext:  -1,
		}
		copy(ebr.PartName[:], []byte(name))

		//escribir EBR en la posicion
		file.Seek(startPosition, 0)
		if err := binary.Write(file, binary.LittleEndian, &ebr); err != nil {
			fmt.Printf("Error al escribir el EBR: %v\n", err)
			return
		}
	}

	var tipoNombre string
	switch tipo {
	case "P":
		tipoNombre = "Primaria"
	case "E":
		tipoNombre = "Extendida"
	case "L":
		tipoNombre = "Lógica"
	}

	fmt.Printf("Partición '%s' de tipo '%s' creada exitosamente en '%s'.\n", name, tipoNombre, diskName)
	fmt.Printf("Tamaño: %d bytes, Ajuste: %s, Posición: %d\n", sizeInBytes, fit, startPosition)

	if err := file.Sync(); err != nil {
		fmt.Printf("Error al sincronizar el archivo: %v\n", err)
		return
	}
}

// validar nombre duplicado
func validatePartitionName(name string, mbr *structs.MBR) error {
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			partitionName := strings.TrimSpace(string(partition.Part_name[:]))
			if partitionName == name {
				return fmt.Errorf("ya existe una partición con el nombre '%s'", name)
			}
		}
	}
	return nil
}

// validar restricciones de particiones
func validatePartitionConstraints(tipo string, mbr *structs.MBR) error {
	primaryCount := 0
	extendedCount := 0

	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			switch partition.Part_type {
			case 'P':
				primaryCount++
			case 'E':
				extendedCount++
			}
		}
	}

	//restriccion de 4 particiones primarias + extendidas
	if tipo == "P" || tipo == "E" {
		if primaryCount+extendedCount >= 4 {
			return fmt.Errorf("no se pueden crear más particiones. Máximo 4 particiones primarias/extendidas")
		}
	}

	//restriccion solo una particion extendida por disco
	if tipo == "E" && extendedCount >= 1 {
		return fmt.Errorf("ya existe una partición extendida en el disco")
	}

	//restriccion no se puede crear particion logica sin extendida
	if tipo == "L" && extendedCount == 0 {
		return fmt.Errorf("no se puede crear una partición lógica sin una partición extendida")
	}

	return nil
}

func validateAvailableSpace(sizeInBytes int64, mbr *structs.MBR, tipo string) error {
	if tipo == "L" {
		return validateLogicalPartitionSpace(sizeInBytes, mbr)
	}

	usedSpace := int64(512) // MBR
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			usedSpace += partition.Part_s
		}
	}

	availableSpace := mbr.Mbr_tamano - usedSpace

	if sizeInBytes > availableSpace {
		return fmt.Errorf("no hay espacio suficiente en el disco. Disponible: %d bytes, Requerido: %d bytes",
			availableSpace, sizeInBytes)
	}

	return nil
}

func validateLogicalPartitionSpace(sizeInBytes int64, mbr *structs.MBR) error {

	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' && partition.Part_type == 'E' {

			if sizeInBytes > partition.Part_s-1024 {
				return fmt.Errorf("no hay espacio suficiente en la partición extendida")
			}
			return nil
		}
	}
	return fmt.Errorf("no se encontró partición extendida")
}

func findFreePartitionSlot(mbr *structs.MBR) int {
	for i := 0; i < 4; i++ {
		if mbr.Mbr_partitions[i].Part_status == '0' {
			return i
		}
	}
	return -1
}

func calculateStartPosition(mbr *structs.MBR, fit string, sizeNeeded int64) int64 {
	switch fit {
	case "FF":
		return calculateFirstFit(mbr, sizeNeeded)
	case "BF":
		return calculateBestFit(mbr, sizeNeeded)
	case "WF":
		return calculateWorstFit(mbr, sizeNeeded)
	default:
		return calculateWorstFit(mbr, sizeNeeded)
	}
}

func calculateBestFit(mbr *structs.MBR, sizeNeeded int64) int64 {

	freeSpaces := getFreeSpaces(mbr)

	if len(freeSpaces) == 0 {
		return int64(512)
	}

	bestStart := int64(-1)
	bestSize := int64(math.MaxInt64)

	for _, space := range freeSpaces {
		if space.Size >= sizeNeeded && space.Size < bestSize {
			bestSize = space.Size
			bestStart = space.Start
		}
	}

	if bestStart == -1 && len(freeSpaces) > 0 {
		return freeSpaces[len(freeSpaces)-1].Start
	}

	if bestStart == -1 {
		return int64(512)
	}

	return bestStart
}

func calculateWorstFit(mbr *structs.MBR, sizeNeeded int64) int64 {

	freeSpaces := getFreeSpaces(mbr)

	if len(freeSpaces) == 0 {
		return int64(512)
	}

	worstStart := int64(-1)
	worstSize := int64(0)

	for _, space := range freeSpaces {
		if space.Size >= sizeNeeded && space.Size > worstSize {
			worstSize = space.Size
			worstStart = space.Start
		}
	}

	if worstStart == -1 && len(freeSpaces) > 0 {
		return freeSpaces[len(freeSpaces)-1].Start
	}

	if worstStart == -1 {
		return int64(512)
	}

	return worstStart
}

type FreeSpace struct {
	Start int64
	Size  int64
}

func getFreeSpaces(mbr *structs.MBR) []FreeSpace {
	var freeSpaces []FreeSpace
	var usedRanges []FreeSpace

	usedRanges = append(usedRanges, FreeSpace{Start: 0, Size: 512})

	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			usedRanges = append(usedRanges, FreeSpace{
				Start: partition.Part_start,
				Size:  partition.Part_s,
			})
		}
	}

	for i := 0; i < len(usedRanges)-1; i++ {
		for j := i + 1; j < len(usedRanges); j++ {
			if usedRanges[i].Start > usedRanges[j].Start {
				usedRanges[i], usedRanges[j] = usedRanges[j], usedRanges[i]
			}
		}
	}

	currentPos := int64(0)

	for _, used := range usedRanges {

		if used.Start > currentPos {
			freeSpaces = append(freeSpaces, FreeSpace{
				Start: currentPos,
				Size:  used.Start - currentPos,
			})
		}

		endPos := used.Start + used.Size
		if endPos > currentPos {
			currentPos = endPos
		}
	}

	if currentPos < mbr.Mbr_tamano {
		freeSpaces = append(freeSpaces, FreeSpace{
			Start: currentPos,
			Size:  mbr.Mbr_tamano - currentPos,
		})
	}

	return freeSpaces
}

func calculateFirstFit(mbr *structs.MBR, sizeNeeded int64) int64 {

	freeSpaces := getFreeSpaces(mbr)

	if len(freeSpaces) == 0 {
		return int64(512)
	}

	for _, space := range freeSpaces {
		if space.Size >= sizeNeeded {
			return space.Start
		}
	}

	return freeSpaces[len(freeSpaces)-1].Start
}

func handleLogicalPartition(mbr *structs.MBR, sizeInBytes int64, fit string, name string, file *os.File) (int64, error) {

	var extendedPartition *structs.Partition
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' && partition.Part_type == 'E' {
			extendedPartition = &partition
			break
		}
	}

	if extendedPartition == nil {
		return 0, fmt.Errorf("no se encontró partición extendida")
	}

	ebrPosition := extendedPartition.Part_start + 1024

	ebr := structs.EBR{
		PartMount: 0,
		PartFit:   fit[0],
		PartStart: ebrPosition,
		PartS:     sizeInBytes,
		PartNext:  -1,
	}
	copy(ebr.PartName[:], []byte(name))

	//escribir EBR
	file.Seek(ebrPosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &ebr); err != nil {
		return 0, fmt.Errorf("error escribiendo EBR: %v", err)
	}

	return ebrPosition, nil
}

func convertSize(size int64, unit string) int64 {
	switch strings.ToUpper(unit) {
	case "K":
		return size * 1024
	case "M":
		return size * 1024 * 1024
	case "B":
		return size
	default:
		return size * 1024
	}
}
