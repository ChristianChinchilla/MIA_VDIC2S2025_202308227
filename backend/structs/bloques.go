package structs

// bloqueCarpeta representa un bloque que tiene informacion de directorios
type BloqueCarpeta struct {
	BContent [4]BContent
}

// bloqueArchivo representa un bloque que tiene datos de archivos
type BloqueArchivo struct {
	BContent [64]byte
}

// bloqueApuntador representa un bloque que tiene punteros a otros bloques
type BloqueApuntador struct {
	BPointers [8]int64
}

// BContent representa una entrada en un directorio
type BContent struct {
	BName  [12]byte
	_      [4]byte
	BInodo int64
}
