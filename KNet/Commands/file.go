package Commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region File help funcs

//region^ FileStorage

// desc: Check and return absolute path
func PrepareFileStorage(fileStorageDir string) (string, error) {
	if fileStorageDir == "" {
		return "", fmt.Errorf("fileStorageDir is empty")
	}

	base := ""
	if exe, err := os.Executable(); err == nil {
		base = filepath.Dir(exe)
	}

	abs := fileStorageDir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(base, abs)
	}
	if resolved, err := filepath.Abs(abs); err == nil {
		abs = resolved
	}

	info, err := os.Stat(abs)
	switch {
	case err == nil:
		if !info.IsDir() {
			return "", fmt.Errorf("path %q is not a directory", abs)
		}
	case os.IsNotExist(err):
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", fmt.Errorf("couldn't create directory %q: %w", abs, err)
		}
	default:
		return "", fmt.Errorf("couldn't stat path %q: %w", abs, err)
	}

	probe := filepath.Join(abs, ".write_probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return "", fmt.Errorf("directory %q is not writable: %w", abs, err)
	}
	if err := os.Remove(probe); err != nil {
		return "", fmt.Errorf("couldn't remove write probe %q: %w", probe, err)
	}

	return abs, nil
}

//endregion^

//region^ File

const fileChunkSize = 4 * 1024

type FileMeta struct {
	FileName   string `json:"FileName"`
	ChunkID    int    `json:"ChunkID"`
	ChunkCount int    `json:"ChunkCount"`
	FileHash   string `json:"FileHash"`
	ChunkHash  string `json:"ChunkHash"`
}

type FileChunk struct {
	Metadata FileMeta
	Chunk    []byte
}

// desc: Checking and slicing a file into chunks
func PrepareFile(filePath string) ([]FileChunk, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("couldn't open the file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("couldn't get the file size: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("the directory is specified, not the file: %s", filePath)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("the specified file is not an ordinary one: %s", filePath)
	}

	name := filepath.Base(filePath)
	total := int(info.Size())
	if total == 0 {
		return nil, fmt.Errorf("the file is empty: %s", filePath)
	}
	chunkCount := (total + fileChunkSize - 1) / fileChunkSize

	//* 1. FileHash — SHA-256 of the entire file
	fileHasher := sha256.New()
	if _, err := io.Copy(fileHasher, f); err != nil {
		return nil, fmt.Errorf("file hashing error: %w", err)
	}
	fileHash := hex.EncodeToString(fileHasher.Sum(nil))

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("file seek error: %w", err)
	}

	//* 2. Slicing into chunks
	chunks := make([]FileChunk, 0, chunkCount)
	buf := make([]byte, fileChunkSize)
	for chunkID := 0; chunkID < chunkCount; chunkID++ {
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("file reading error: %w", err)
		}
		if n == 0 {
			break
		}

		chunk := make([]byte, n)
		copy(chunk, buf[:n])

		chunkSum := sha256.Sum256(chunk)

		chunks = append(chunks, FileChunk{
			Metadata: FileMeta{
				FileName:   name,
				ChunkID:    chunkID,
				ChunkCount: chunkCount,
				FileHash:   fileHash,
				ChunkHash:  hex.EncodeToString(chunkSum[:]),
			},
			Chunk: chunk,
		})
	}

	return chunks, nil
}

//endregion^

//region^ Package to Chunk

func PackageToChunk(pkg *knetpb.Package) (FileChunk, error) {
	var meta FileMeta
	if err := json.Unmarshal([]byte(pkg.Metadata), &meta); err != nil {

		return FileChunk{}, fmt.Errorf("FILE package metadata could not be parsed: %s", err.Error())
	}

	//* Checking the hash of the chunk
	chunkSum := sha256.Sum256(pkg.GetBlob())
	if meta.ChunkHash != "" && hex.EncodeToString(chunkSum[:]) != meta.ChunkHash {
		return FileChunk{}, fmt.Errorf("Chunk hash mismatch for %s (chunk %d), skipped", meta.FileName, meta.ChunkID)
	}

	chunk := FileChunk{
		Metadata: meta,
		Chunk:    pkg.GetBlob(),
	}

	return chunk, nil
}

//endregion^

//endregion

//region File

func FileHelp() string {
	return "/file <peerID> <path> — Send a file by chunks"
}

func FileCheck(i Interface, cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
	if len(cmd.Args) != 2 {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			"Incorrect number of arguments: expected /file <peerID> <path>",
		), false
	}

	id := cmd.Args[0]
	path := cmd.Args[1]

	if id.ID == "" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The first argument must be an ID: $..."), false
	}

	if path.Str == "" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The second argument must be a string: \"...\""), false
	}

	if id.Value() == "" {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			"The peerID cannot be empty",
		), false
	}

	if path.Value() == "" {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			"path cannot be empty",
		), false
	}

	return nil, true
}

func File(i Interface, cmd *KCommandParser.Command, fileStorageDir string) *KCommandParser.Response {
	to := cmd.Args[0].Value()
	if !i.KNetCore.IsPeerKnown(to) {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			fmt.Sprintf("%s: unknown peer %q", cmd.Name, to),
		)
	}
	path := cmd.Args[1].Value()

	if !filepath.IsAbs(path) {
		base, err := PrepareFileStorage(fileStorageDir)
		if err != nil {
			return Proto.NewResponse(
				cmd.ID,
				knetpb.RecordLevel_ERROR,
				"PrepareFileStorage: "+err.Error(),
			)
		}
		path = filepath.Join(base, path)
	}

	chunks, err := PrepareFile(path)
	if err != nil {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			"PrepareFile: "+err.Error(),
		)
	}

	from := i.KNetCore.GetMyNode().Id

	sent := 0
	for _, chunk := range chunks {
		raw, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return Proto.NewResponse(
				cmd.ID,
				knetpb.RecordLevel_ERROR,
				"Metadata serialization error: "+err.Error(),
			)
		}

		i.KNetCore.Write(&knetpb.Package{
			From:     from,
			To:       to,
			Type:     knetpb.PackageType_FILE,
			DataType: knetpb.DataType_BYTES,
			Metadata: string(raw),
			Payload:  &knetpb.Package_Blob{Blob: chunk.Chunk},
		})
		sent++
	}

	return Proto.NewResponse(
		cmd.ID,
		knetpb.RecordLevel_INFO,
		fmt.Sprintf("file %q (%d chunks) sent to %s", filepath.Base(path), sent, to),
	)
}

//endregion
