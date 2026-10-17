package ncmcrypt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"git.taurusxin.com/taurusxin/ncmdump-go/utils"
	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/flacvorbis"
	"github.com/go-flac/go-flac"
)

type NcmFormat = string

const (
	Mp3 NcmFormat = "mp3"
)
const (
	Flac NcmFormat = "flac"
)

type NeteaseCloudMusic struct {
	Context                                                   context.Context
	CoverWarning                                              string
	ReadTime, DecryptTime, WriteTime, CoverTime, MetadataTime time.Duration

	sCoreKey   [17]byte
	sModifyKey [17]byte
	mPng       [8]byte

	mFilePath string

	mDumpFilePath string
	mFormat       NcmFormat
	mImageData    []byte
	mFileStream   *os.File
	mKeyBox       [256]byte
	mMetadata     *NeteaseClousMusicMetadata
	mAlbumPicUrl  string
}

func (ncm *NeteaseCloudMusic) read(buffer *[]byte, size int) int {
	if len(*buffer) < size {
		*buffer = make([]byte, size)
	}
	res, err := io.ReadFull(ncm.mFileStream, (*buffer)[:size])
	if err != nil {
		return 0
	}
	return res
}

func (ncm *NeteaseCloudMusic) openFile() bool {
	file, err := os.Open(ncm.mFilePath)
	if err != nil {
		return false
	}
	ncm.mFileStream = file
	return true
}

func (ncm *NeteaseCloudMusic) isNcmFile() bool {
	header := make([]byte, 4)

	// check magic header 4E455443 4D414446
	if ncm.read(&header, 4) != 4 {
		return false
	}
	if int(binary.LittleEndian.Uint32(header)) != 0x4E455443 {
		return false
	}
	if ncm.read(&header, 4) != 4 {
		return false
	}

	if int(binary.LittleEndian.Uint32(header)) != 0x4D414446 {
		return false
	}

	return true
}

func (ncm *NeteaseCloudMusic) buildKeyBox(key []byte, keyLen int) {
	for i := 0; i < 256; i++ {
		ncm.mKeyBox[i] = byte(i)
	}

	var swap uint8 = 0
	var c uint8 = 0
	var lastByte uint8 = 0
	var keyOffset uint8 = 0

	for i := 0; i < 256; i++ {
		swap = ncm.mKeyBox[i]
		c = (swap + lastByte + key[keyOffset]) & 0xff
		keyOffset++
		if int(keyOffset) >= keyLen {
			keyOffset = 0
		}
		ncm.mKeyBox[i] = ncm.mKeyBox[c]
		ncm.mKeyBox[c] = swap
		lastByte = c
	}
}

func (ncm *NeteaseCloudMusic) mimeType() string {
	if bytes.HasPrefix(ncm.mImageData, ncm.mPng[:]) {
		return "image/png"
	}
	return "image/jpeg"
}

// Dump encrypted ncm file to normal music file. If `targetDir` is "", the converted file will be saved to the original directory.
func (ncm *NeteaseCloudMusic) Dump(targetDir string) (bool, error) {
	ncm.mDumpFilePath = ncm.mFilePath
	var output *os.File
	defer func() {
		if output != nil {
			output.Close()
		}
	}()
	var mask [256]byte
	for i := range mask {
		j := (i + 1) & 255
		mask[i] = ncm.mKeyBox[(ncm.mKeyBox[j]+ncm.mKeyBox[(int(ncm.mKeyBox[j])+j)&255])&255]
	}
	buffer := make([]byte, 256*1024)
	offset := 0
	for {
		if ncm.Context != nil {
			if err := ncm.Context.Err(); err != nil {
				return false, err
			}
		}
		t := time.Now()
		count, err := ncm.mFileStream.Read(buffer)
		ncm.ReadTime += time.Since(t)
		if count > 0 {
			t = time.Now()
			for i := 0; i < count; i++ {
				buffer[i] ^= mask[(offset+i)&255]
			}
			offset += count
			ncm.DecryptTime += time.Since(t)
			if output == nil {
				if count >= 3 && string(buffer[:3]) == "ID3" {
					ncm.mFormat = Mp3
				} else if count >= 4 && string(buffer[:4]) == "fLaC" {
					ncm.mFormat = Flac
				} else {
					return false, fmt.Errorf("unknown audio format")
				}
				ncm.mDumpFilePath = utils.ReplaceExtension(ncm.mFilePath, "."+ncm.mFormat)
				if targetDir != "" {
					ncm.mDumpFilePath = filepath.Join(targetDir, filepath.Base(ncm.mDumpFilePath))
				}
				output, err = os.OpenFile(ncm.mDumpFilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err != nil {
					return false, err
				}
			}
			t = time.Now()
			written, e := output.Write(buffer[:count])
			ncm.WriteTime += time.Since(t)
			if e != nil {
				return false, e
			}
			if written != count {
				return false, io.ErrShortWrite
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
	}
	if output == nil {
		return false, io.ErrUnexpectedEOF
	}
	return true, output.Close()
}

// FixMetadata writes text tags and embedded cover only. No network access.
// The argument is retained for API compatibility and is ignored.
func (ncm *NeteaseCloudMusic) FixMetadata(_ bool) (bool, error) {
	defer func(t time.Time) { ncm.MetadataTime = time.Since(t) }(time.Now())
	if ncm.mMetadata == nil {
		return true, nil
	}
	switch ncm.mFormat {
	case Mp3:
		audioFile, err := id3v2.Open(ncm.mDumpFilePath, id3v2.Options{Parse: true})
		if err != nil {
			return false, err
		}
		defer audioFile.Close()
		audioFile.SetDefaultEncoding(id3v2.EncodingUTF8)
		audioFile.SetTitle(ncm.mMetadata.mName)
		audioFile.SetArtist(ncm.mMetadata.mArtist)
		audioFile.SetAlbum(ncm.mMetadata.mAlbum)

		if len(ncm.mImageData) > 0 {
			pic := id3v2.PictureFrame{
				Encoding:    id3v2.EncodingUTF8,
				MimeType:    ncm.mimeType(),
				PictureType: id3v2.PTFrontCover,
				Description: "",
				Picture:     ncm.mImageData,
			}
			audioFile.AddAttachedPicture(pic)
		}

		err = audioFile.Save()
		if err != nil {
			return false, err
		}
	case Flac:
		audioFile, err := flac.ParseFile(ncm.mDumpFilePath)
		if err != nil {
			return false, err
		}
		if len(ncm.mImageData) > 0 {
			pic, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "",
				ncm.mImageData, ncm.mimeType())
			if err != nil {
				return false, err
			}
			pictureMeta := pic.Marshal()
			audioFile.Meta = append(audioFile.Meta, &pictureMeta)
		}

		var cmts *flacvorbis.MetaDataBlockVorbisComment
		var cmtIdx int
		for idx, meta := range audioFile.Meta {
			if meta.Type == flac.VorbisComment {
				cmts, err = flacvorbis.ParseFromMetaDataBlock(*meta)
				cmtIdx = idx
				if err != nil {
					return false, err
				}
			}
		}
		if cmts == nil {
			cmts = flacvorbis.New()
		}

		// flac 可能自带元数据 当且仅当没有该项时才向目标添加元数据
		if res, _ := cmts.Get(flacvorbis.FIELD_TITLE); len(res) == 0 {
			_ = cmts.Add(flacvorbis.FIELD_TITLE, ncm.mMetadata.mName)
		}
		if res, _ := cmts.Get(flacvorbis.FIELD_ARTIST); len(res) == 0 {
			_ = cmts.Add(flacvorbis.FIELD_ARTIST, ncm.mMetadata.mArtist)
		}
		if res, _ := cmts.Get(flacvorbis.FIELD_ALBUM); len(res) == 0 {
			_ = cmts.Add(flacvorbis.FIELD_ALBUM, ncm.mMetadata.mAlbum)
		}

		cmtsmeta := cmts.Marshal()

		if cmtIdx > 0 {
			audioFile.Meta[cmtIdx] = &cmtsmeta
		} else {
			audioFile.Meta = append(audioFile.Meta, &cmtsmeta)
		}

		err = audioFile.Save(ncm.mDumpFilePath)

		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// GetDumpFilePath returns the absolute path of dumped music file
func (ncm *NeteaseCloudMusic) GetDumpFilePath() string {
	path, err := filepath.Abs(ncm.mDumpFilePath)
	if err != nil {
		return ncm.mDumpFilePath
	}
	return path
}

// NewNeteaseCloudMusic returns a new NeteaseCloudMusic instance, if the format of the file is incorrect, the error will be returned.
func NewNeteaseCloudMusic(filePath string) (_ *NeteaseCloudMusic, resultErr error) {
	ncm := &NeteaseCloudMusic{
		sCoreKey:   [17]byte{0x68, 0x7A, 0x48, 0x52, 0x41, 0x6D, 0x73, 0x6F, 0x35, 0x6B, 0x49, 0x6E, 0x62, 0x61, 0x78, 0x57, 0},
		sModifyKey: [17]byte{0x23, 0x31, 0x34, 0x6C, 0x6A, 0x6B, 0x5F, 0x21, 0x5C, 0x5D, 0x26, 0x30, 0x55, 0x3C, 0x27, 0x28, 0},
		mPng:       [8]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},

		mFilePath: filePath,
	}

	if !ncm.openFile() {
		return nil, fmt.Errorf("open file failed")
	}

	defer func() {
		if r := recover(); r != nil {
			resultErr = fmt.Errorf("invalid NCM: %v", r)
		}
		if resultErr != nil {
			ncm.Close()
		}
	}()
	if !ncm.isNcmFile() {
		return nil, fmt.Errorf("not a ncm file")
	}

	// actually this 2 bytes is the version, now we just skip it
	if _, err := ncm.mFileStream.Seek(2, 1); err != nil {
		return nil, fmt.Errorf("seek version failed")
	}

	// the length of the RC4 key, encrypted by AES128
	n := make([]byte, 4)

	if ncm.read(&n, len(n)) != 4 {
		return nil, fmt.Errorf("read key len failed")
	}

	keyLen := int(binary.LittleEndian.Uint32(n))

	if keyLen < 16 || keyLen > 1024*1024 || keyLen%16 != 0 {
		return nil, fmt.Errorf("invalid key length")
	}
	keydata := make([]byte, keyLen)
	if ncm.read(&keydata, keyLen) != keyLen {
		return nil, io.ErrUnexpectedEOF
	}

	for i := range keydata {
		keydata[i] ^= 0x64
	}

	mKeyData, err := utils.AesEcbDecrypt(ncm.sCoreKey[:16], keydata)

	if err != nil {
		return nil, fmt.Errorf("decrypt key failed")
	}

	if len(mKeyData) <= 17 {
		return nil, fmt.Errorf("short decrypted key")
	}
	// build the key box
	ncm.buildKeyBox(mKeyData[17:], len(mKeyData)-17)

	if ncm.read(&n, len(n)) != 4 {
		return nil, fmt.Errorf("read metadata len failed")
	}

	metadataLen := int(binary.LittleEndian.Uint32(n))

	if metadataLen <= 0 {
		// process meta here
		ncm.mMetadata = nil
	} else {
		// read metadata
		if metadataLen < 22 || metadataLen > 16*1024*1024 {
			return nil, fmt.Errorf("invalid metadata length")
		}
		modifyData := make([]byte, metadataLen)
		if ncm.read(&modifyData, metadataLen) != metadataLen {
			return nil, io.ErrUnexpectedEOF
		}

		for i := range modifyData {
			modifyData[i] ^= 0x63
		}

		// escape `163 key(Don't modify):`
		swapModifyData := string(modifyData[22:])

		modifyOutData, err := base64.StdEncoding.DecodeString(swapModifyData)
		if err != nil {
			return nil, fmt.Errorf("base64 metadata: %w", err)
		}

		modifyDecryptData, err := utils.AesEcbDecrypt(ncm.sModifyKey[:16], modifyOutData)

		if err != nil {
			return nil, fmt.Errorf("decrypt metadata: %w", err)
		}

		if len(modifyDecryptData) < 6 {
			return nil, fmt.Errorf("short metadata")
		}
		// escape `music:`
		mMetadataString := string(modifyDecryptData[6:])

		// extract the album pic url
		ncm.mAlbumPicUrl = GetAlbumPicUrl(mMetadataString)

		ncm.mMetadata = NewNeteaseCloudMusicMetadata(mMetadataString)
	}

	// skip the 5 bytes gap
	if _, err := ncm.mFileStream.Seek(5, 1); err != nil {
		return nil, fmt.Errorf("seek gap failed")
	}

	// read the cover frame
	coverFrameLen := make([]byte, 4)

	if ncm.read(&coverFrameLen, len(coverFrameLen)) != 4 {
		return nil, fmt.Errorf("read cover frame len failed")
	}

	if ncm.read(&n, len(n)) != 4 {
		return nil, fmt.Errorf("read cover frame data len failed")
	}

	coverFrameLenInt := int(binary.LittleEndian.Uint32(coverFrameLen))
	coverFrameDataLen := int(binary.LittleEndian.Uint32(n))

	if coverFrameDataLen > 16*1024*1024 || coverFrameLenInt < coverFrameDataLen {
		return nil, fmt.Errorf("invalid cover length")
	}
	if coverFrameDataLen > 0 {
		if ncm.read(&ncm.mImageData, coverFrameDataLen) != coverFrameDataLen {
			return nil, io.ErrUnexpectedEOF
		}
	}

	ncm.mFileStream.Seek(int64(coverFrameLenInt)-int64(coverFrameDataLen), 1)

	return ncm, nil
}

func (ncm *NeteaseCloudMusic) Close() error { return ncm.mFileStream.Close() }
