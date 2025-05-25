package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const version = "0.2.0-ffprobe"

// AudioInfo represents metadata about an audio file
type AudioInfo struct {
	FilePath         string            `json:"file_path"`
	FileSize         int64             `json:"file_size"`
	DurationSeconds  float64           `json:"duration_seconds"`
	BitRate          int64             `json:"bit_rate"`
	SampleRate       int               `json:"sample_rate"`
	Channels         int               `json:"channels"`
	CodecName        string            `json:"codec_name"`
	CodecLongName    string            `json:"codec_long_name"`
	FormatName       string            `json:"format_name"`
	FormatLongName   string            `json:"format_long_name"`
	HasVideo         bool              `json:"has_video"`
	Metadata         map[string]string `json:"metadata"`
	ProcessingTimeMs int64             `json:"processing_time_ms"`
}

// FFProbeOutput represents the JSON output from ffprobe
type FFProbeOutput struct {
	Format  FFProbeFormat   `json:"format"`
	Streams []FFProbeStream `json:"streams"`
}

// FFProbeFormat represents format information from ffprobe
type FFProbeFormat struct {
	Filename       string            `json:"filename"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	Duration       string            `json:"duration"`
	Size           string            `json:"size"`
	BitRate        string            `json:"bit_rate"`
	Tags           map[string]string `json:"tags"`
}

// FFProbeStream represents stream information from ffprobe
type FFProbeStream struct {
	CodecName     string `json:"codec_name"`
	CodecLongName string `json:"codec_long_name"`
	CodecType     string `json:"codec_type"`
	SampleRate    string `json:"sample_rate"`
	Channels      int    `json:"channels"`
	BitRate       string `json:"bit_rate"`
}

// Result represents the processing result for a file
type Result struct {
	Info  *AudioInfo
	Error error
}

var (
	concurrency  int
	outputFormat string
	outputFile   string
	recursive    bool
	showVersion  bool
	quiet        bool
)

func main() {
	// Parse command line flags
	flag.IntVar(&concurrency, "j", runtime.NumCPU()*2, "並行処理数")
	flag.StringVar(&outputFormat, "format", "text", "出力形式 (text/json)")
	jsonFlag := flag.Bool("json", false, "JSON形式で出力 (--format jsonのショートカット)")
	flag.StringVar(&outputFile, "o", "", "出力ファイルパス")
	flag.BoolVar(&recursive, "r", false, "ディレクトリを再帰的に検索")
	flag.BoolVar(&showVersion, "version", false, "バージョン情報を表示")
	flag.BoolVar(&quiet, "q", false, "プログレス表示を無効化")
	flag.Parse()

	if showVersion {
		fmt.Printf("Audio Probe Go FFprobe v%s\n", version)
		os.Exit(0)
	}

	if *jsonFlag {
		outputFormat = "json"
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// Check if ffprobe is available
	if !checkFFProbe() {
		log.Fatal("ffprobeが見つかりません。FFmpegをインストールしてください。")
	}

	// Collect audio files
	audioFiles, err := collectAudioFiles(args, recursive)
	if err != nil {
		log.Fatalf("ファイル収集エラー: %v", err)
	}

	if len(audioFiles) == 0 {
		log.Println("音声ファイルが見つかりませんでした")
		os.Exit(0)
	}

	// Process files
	results := processFiles(audioFiles, concurrency)

	// Output results
	if err := outputResults(results, outputFormat, outputFile); err != nil {
		log.Fatalf("出力エラー: %v", err)
	}
}

func checkFFProbe() bool {
	cmd := exec.Command("ffprobe", "-version")
	err := cmd.Run()
	return err == nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "使用方法: %s [オプション] <ファイル/ディレクトリ>...\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "\nオプション:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "\n例:")
	fmt.Fprintf(os.Stderr, "  %s audio.mp3\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -j 100 /path/to/music/\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --json -r /path/to/music/ > results.json\n", os.Args[0])
}

func collectAudioFiles(paths []string, recursive bool) ([]string, error) {
	var audioFiles []string
	audioExtensions := map[string]bool{
		".mp3": true, ".wav": true, ".flac": true, ".aac": true,
		".ogg": true, ".m4a": true, ".wma": true, ".opus": true,
		".mp2": true, ".ac3": true, ".dts": true, ".ape": true,
		".aiff": true, ".au": true, ".ra": true, ".amr": true,
		".webm": true, ".mkv": true, ".m4b": true, ".m4p": true,
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("パスが存在しません: %s", path)
		}

		if info.IsDir() {
			// ディレクトリ処理
			if recursive {
				err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !info.IsDir() {
						ext := strings.ToLower(filepath.Ext(p))
						if audioExtensions[ext] {
							audioFiles = append(audioFiles, p)
						}
					}
					return nil
				})
			} else {
				entries, err := os.ReadDir(path)
				if err != nil {
					return nil, err
				}
				for _, entry := range entries {
					if !entry.IsDir() {
						fullPath := filepath.Join(path, entry.Name())
						ext := strings.ToLower(filepath.Ext(fullPath))
						if audioExtensions[ext] {
							audioFiles = append(audioFiles, fullPath)
						}
					}
				}
			}
			if err != nil {
				return nil, err
			}
		} else {
			// ファイル処理
			ext := strings.ToLower(filepath.Ext(path))
			if audioExtensions[ext] {
				audioFiles = append(audioFiles, path)
			}
		}
	}

	return audioFiles, nil
}

func processFiles(files []string, maxConcurrency int) []Result {
	// CPU制限による並行数調整
	cpuLimit := runtime.NumCPU() * 12
	if maxConcurrency > cpuLimit {
		log.Printf("警告: 並行数 %d はCPUコア数の12倍 (%d) を超えています。調整します。", maxConcurrency, cpuLimit)
		maxConcurrency = cpuLimit
	}

	fmt.Printf("🎵 Audio Probe Go FFprobe - 高性能音声ファイル解析ツール (v%s)\n", version)
	fmt.Println("FFprobeを使用して実際の音声ファイル情報を解析します")
	log.Printf("Found %d audio files to process", len(files))
	log.Printf("Processing %d files with max %d concurrent operations", len(files), maxConcurrency)

	startTime := time.Now()
	results := make([]Result, len(files))
	
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxConcurrency)
	var processed int32
	
	// プログレス表示
	var progressWg sync.WaitGroup
	progressDone := make(chan bool)
	
	if !quiet {
		progressWg.Add(1)
		go func() {
			defer progressWg.Done()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			
			for {
				select {
				case <-ticker.C:
					p := atomic.LoadInt32(&processed)
					progress := float64(p) / float64(len(files)) * 100
					fmt.Printf("\r  [%.0f%%] %d/%d files processed", progress, p, len(files))
				case <-progressDone:
					fmt.Printf("\r  [100%%] %d/%d files processed ✓      \n", len(files), len(files))
					return
				}
			}
		}()
	}

	for i, file := range files {
		wg.Add(1)
		go func(index int, filePath string) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			info, err := analyzeFileWithFFProbe(filePath)
			results[index] = Result{Info: info, Error: err}
			
			atomic.AddInt32(&processed, 1)
		}(i, file)
	}

	wg.Wait()
	close(progressDone)
	progressWg.Wait()

	elapsed := time.Since(startTime)
	log.Printf("Processing completed in %.2fs", elapsed.Seconds())

	// カウント成功/失敗
	var successCount, failCount int
	for _, result := range results {
		if result.Error == nil {
			successCount++
		} else {
			failCount++
		}
	}
	log.Printf("Successfully processed: %d", successCount)
	if failCount > 0 {
		log.Printf("Failed: %d", failCount)
	}

	return results
}

func analyzeFileWithFFProbe(filePath string) (*AudioInfo, error) {
	startTime := time.Now()

	// ファイル情報取得
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("ファイルが見つかりません: %v", err)
	}

	// ffprobeコマンドを実行
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe実行エラー: %v", err)
	}

	// JSONをパース
	var probeData FFProbeOutput
	if err := json.Unmarshal(output, &probeData); err != nil {
		return nil, fmt.Errorf("JSONパースエラー: %v", err)
	}

	// 音声ストリームを探す
	var audioStream *FFProbeStream
	hasVideo := false

	for _, stream := range probeData.Streams {
		if stream.CodecType == "audio" && audioStream == nil {
			audioStream = &stream
		} else if stream.CodecType == "video" {
			hasVideo = true
		}
	}

	if audioStream == nil {
		return nil, fmt.Errorf("音声ストリームが見つかりません")
	}

	// 値の変換
	duration, _ := strconv.ParseFloat(probeData.Format.Duration, 64)
	bitRate, _ := strconv.ParseInt(probeData.Format.BitRate, 10, 64)
	sampleRate, _ := strconv.Atoi(audioStream.SampleRate)
	streamBitRate, _ := strconv.ParseInt(audioStream.BitRate, 10, 64)

	// ビットレートが0の場合、ストリームのビットレートを使用
	if bitRate == 0 && streamBitRate > 0 {
		bitRate = streamBitRate
	}

	// メタデータ取得
	metadata := make(map[string]string)
	for k, v := range probeData.Format.Tags {
		metadata[strings.ToLower(k)] = v
	}

	// AudioInfo構築
	info := &AudioInfo{
		FilePath:         filePath,
		FileSize:         fileInfo.Size(),
		DurationSeconds:  duration,
		BitRate:          bitRate,
		SampleRate:       sampleRate,
		Channels:         audioStream.Channels,
		CodecName:        audioStream.CodecName,
		CodecLongName:    audioStream.CodecLongName,
		FormatName:       probeData.Format.FormatName,
		FormatLongName:   probeData.Format.FormatLongName,
		HasVideo:         hasVideo,
		Metadata:         metadata,
		ProcessingTimeMs: time.Since(startTime).Milliseconds(),
	}

	// メタデータが空の場合、デフォルト値を設定
	if _, ok := metadata["title"]; !ok {
		baseName := filepath.Base(filePath)
		info.Metadata["title"] = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	}
	if _, ok := metadata["artist"]; !ok {
		info.Metadata["artist"] = "Unknown Artist"
	}
	if _, ok := metadata["album"]; !ok {
		info.Metadata["album"] = "Unknown Album"
	}

	return info, nil
}

func outputResults(results []Result, format string, outputFile string) error {
	var output *os.File
	var err error

	if outputFile != "" {
		output, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("出力ファイルの作成に失敗: %v", err)
		}
		defer output.Close()
		log.Printf("結果を %s に書き込みました", outputFile)
	} else {
		output = os.Stdout
	}

	writer := bufio.NewWriter(output)
	defer writer.Flush()

	switch format {
	case "json":
		return outputJSON(writer, results)
	default:
		return outputText(writer, results)
	}
}

func outputJSON(w *bufio.Writer, results []Result) error {
	// 成功した結果のみを含む
	var validResults []*AudioInfo
	for _, result := range results {
		if result.Error == nil && result.Info != nil {
			validResults = append(validResults, result.Info)
		}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(validResults)
}

func outputText(w *bufio.Writer, results []Result) error {
	fmt.Fprintln(w, "=== 音声ファイル分析結果 ===")
	
	// 統計情報
	var totalDuration float64
	var totalSize int64
	var successCount int
	
	for _, result := range results {
		if result.Error == nil && result.Info != nil {
			totalDuration += result.Info.DurationSeconds
			totalSize += result.Info.FileSize
			successCount++
		}
	}
	
	fmt.Fprintf(w, "成功: %d, 失敗: %d\n", successCount, len(results)-successCount)
	fmt.Fprintf(w, "総継続時間: %s\n", formatDuration(totalDuration))
	fmt.Fprintf(w, "総サイズ: %s\n", formatBytes(totalSize))
	
	// 個別結果
	for _, result := range results {
		if result.Error != nil {
			fmt.Fprintf(w, "\n❌ エラー: %v\n", result.Error)
			continue
		}
		
		info := result.Info
		fmt.Fprintf(w, "\n📁 ファイル: %s\n", info.FilePath)
		fmt.Fprintf(w, "   サイズ: %s\n", formatBytes(info.FileSize))
		fmt.Fprintf(w, "   継続時間: %s\n", formatDuration(info.DurationSeconds))
		fmt.Fprintf(w, "   ビットレート: %s\n", formatBitRate(info.BitRate))
		fmt.Fprintf(w, "   サンプルレート: %d Hz\n", info.SampleRate)
		fmt.Fprintf(w, "   チャンネル数: %d\n", info.Channels)
		fmt.Fprintf(w, "   コーデック: %s (%s)\n", info.CodecName, info.CodecLongName)
		fmt.Fprintf(w, "   フォーマット: %s (%s)\n", info.FormatName, info.FormatLongName)
		fmt.Fprintf(w, "   動画含む: %s\n", formatBool(info.HasVideo))
		fmt.Fprintf(w, "   処理時間: %dms\n", info.ProcessingTimeMs)
		
		if len(info.Metadata) > 0 {
			fmt.Fprintln(w, "   メタデータ:")
			for key, value := range info.Metadata {
				if value != "" {
					fmt.Fprintf(w, "     %s: %s\n", key, value)
				}
			}
		}
	}
	
	return nil
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

func formatDuration(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	
	if hours > 0 {
		return fmt.Sprintf("%d時間%d分%d秒", hours, minutes, secs)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分%d秒", minutes, secs)
	}
	return fmt.Sprintf("%.1f秒", seconds)
}

func formatBitRate(bitRate int64) string {
	if bitRate >= 1000000 {
		return fmt.Sprintf("%.1f Mbps", float64(bitRate)/1000000)
	} else if bitRate >= 1000 {
		return fmt.Sprintf("%d kbps", bitRate/1000)
	}
	return fmt.Sprintf("%d bps", bitRate)
}

func formatBool(b bool) string {
	if b {
		return "はい"
	}
	return "いいえ"
}
