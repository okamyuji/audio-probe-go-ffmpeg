# Audio Probe Go FFmpeg

高性能音声ファイル解析ツール - FFmpeg統合版

## 概要

このツールは、FFmpegのネイティブバインディングを使用して、音声ファイルの詳細なメタデータを高速に解析します。Rustの実装に匹敵する性能を目指して設計されています。

## 特徴

- 🚀 **高性能並行処理**: Goroutineによる効率的な並列処理
- 🎵 **FFmpeg統合**: go-astiavを使用したネイティブFFmpegバインディング
- 📊 **包括的な解析**: 実際の音声ファイルメタデータを正確に取得
- ⚡ **2000ファイル同時処理対応**: 大量ファイル処理に最適化
- 🔧 **柔軟な出力形式**: JSON、テキスト、ファイル出力対応

## 必要要件

- Go 1.24+
- FFmpeg 7.0 (ライブラリとヘッダーファイル)
- pkg-config

### FFmpegインストール

macOS

```bash
brew install ffmpeg pkg-config
```

Ubuntu/Debian

```bash
sudo apt update
sudo apt install ffmpeg libavformat-dev libavcodec-dev libavutil-dev libswscale-dev pkg-config
```

## インストール

```bash
git clone https://github.com/okamyuji/audio-probe-go-ffmpeg
cd audio-probe-go-ffmpeg
go mod download
go build -o audio-probe-ffmpeg cmd/audio-probe-ffmpeg/main.go
```

## 使用方法

```bash
# 基本的な使用
./audio-probe-ffmpeg audio_file.mp3

# ディレクトリ内のすべての音声ファイルを解析
./audio-probe-ffmpeg /path/to/music/

# 高並行処理（100同時実行）
./audio-probe-ffmpeg -j 100 /path/to/music/

# JSON出力
./audio-probe-ffmpeg --json /path/to/music/ > results.json

# ファイル出力
./audio-probe-ffmpeg -o analysis_report.txt /path/to/music/

# 再帰的検索
./audio-probe-ffmpeg -r /path/to/music/
```

## パフォーマンス

- 2000ファイルの同時処理に対応
- メモリ効率的なストリーミング処理
- CPUコア数に基づく自動並行数調整

## ライセンス

MIT License
