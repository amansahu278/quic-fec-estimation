package main

import (
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"runtime"
	"time"

	rs "github.com/klauspost/reedsolomon"
	rq "github.com/xssnick/raptorq"
)

// ---------------- XOR ----------------

func EncodeXOR(payload []byte, symbolSize, parity int) ([][]byte, error) {
	k := int(math.Ceil(float64(len(payload)) / float64(symbolSize)))
	parities := make([][]byte, parity)
	for p := 0; p < parity; p++ {
		buf := make([]byte, symbolSize)
		for i := 0; i < k; i++ {
			start := i * symbolSize
			end := start + symbolSize
			if end > len(payload) {
				end = len(payload)
			}
			for j := 0; j < end-start; j++ {
				buf[j] ^= payload[start+j]
			}
		}
		parities[p] = buf
	}
	return parities, nil
}

func DecodeXOR(dataShards [][]byte, lostIndices []int) error {
	if len(lostIndices) == 0 {
		return nil
	}
	k := len(dataShards)
	symbolSize := len(dataShards[0])
	recoverBuf := make([]byte, symbolSize)
	for i := 0; i < k; i++ {
		if i == lostIndices[0] {
			continue
		}
		for j := 0; j < symbolSize; j++ {
			recoverBuf[j] ^= dataShards[i][j]
		}
	}
	copy(dataShards[lostIndices[0]], recoverBuf)
	return nil
}

// ---------------- Reed-Solomon ----------------

func EncodeRS(payload []byte, symbolSize, parity int) ([][]byte, error) {
	dataShards := int(math.Ceil(float64(len(payload)) / float64(symbolSize)))
	parityShards := parity
	enc, err := rs.New(dataShards, parityShards)
	if err != nil {
		return nil, err
	}
	shards := make([][]byte, dataShards+parityShards)
	for i := 0; i < dataShards; i++ {
		start := i * symbolSize
		end := start + symbolSize
		if end > len(payload) {
			end = len(payload)
		}
		shard := make([]byte, symbolSize)
		copy(shard, payload[start:end])
		shards[i] = shard
	}
	for i := dataShards; i < dataShards+parityShards; i++ {
		shards[i] = make([]byte, symbolSize)
	}
	err = enc.Encode(shards)
	return shards, err
}

func DecodeRS(shards [][]byte, missing int) error {
	dataShards := len(shards) - missing
	parityShards := missing
	dec, err := rs.New(dataShards, parityShards)
	if err != nil {
		return err
	}
	for i := 0; i < missing; i++ {
		shards[i] = nil
	}
	ok, err := dec.Verify(shards)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return dec.Reconstruct(shards)
}

// ---------------- RaptorQ ----------------

func EncodeRaptorQ(payload []byte, symbolSize, parity int) ([][]byte, error) {
	obj := rq.NewEncoder(payload, uint16(symbolSize))
	totalSymbols := int(math.Ceil(float64(len(payload)) / float64(symbolSize)))
	total := totalSymbols + parity
	out := make([][]byte, total)
	for i := 0; i < total; i++ {
		sym, err := obj.Encode(uint32(i))
		if err != nil {
			return nil, err
		}
		out[i] = sym
	}
	return out, nil
}

func DecodeRaptorQ(encoded [][]byte, symbolSize int, totalSymbols int) error {
	payloadSize := totalSymbols * symbolSize
	decoder := rq.NewDecoder(uint64(payloadSize), uint16(symbolSize))
	for i, sym := range encoded {
		err := decoder.AddSymbol(sym, uint32(i))
		if err != nil {
			return err
		}
		if decoder.IsDecoded() {
			break
		}
	}
	_, err := decoder.Decode()
	return err
}

// ---------------- Result structs ----------------

type Result struct {
	Algo         string
	PayloadBytes int
	SymbolSize   int
	Parity       int
	MeanEncSec   float64
	MeanDecSec   float64
	EncPerByte   float64
	DecPerByte   float64
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, v := range xs {
		sum += v
	}
	return sum / float64(len(xs))
}

func bench(algo string, encode func([]byte, int, int) ([][]byte, error),
	decode func([][]byte, int) error, payloadB, symbolSize, parity, iters int) Result {

	payload := make([]byte, payloadB)
	rand.Read(payload)

	var encTimes, decTimes []float64
	for i := 0; i < iters; i++ {
		start := time.Now()
		shards, err := encode(payload, symbolSize, parity)
		if err != nil {
			fmt.Printf("%s encode error: %v\n", algo, err)
			return Result{}
		}
		encTimes = append(encTimes, time.Since(start).Seconds())

		if decode != nil {
			start = time.Now()
			err = decode(shards, 1)
			if err != nil {
				fmt.Printf("%s decode error: %v\n", algo, err)
				return Result{}
			}
			decTimes = append(decTimes, time.Since(start).Seconds())
		}
	}

	meanEnc := mean(encTimes)
	meanDec := mean(decTimes)
	tByteEnc := meanEnc / float64(payloadB)
	tByteDec := meanDec / float64(payloadB)

	fmt.Printf("%s | payload=%dB sym=%dB p=%d | Enc=%.6fs (%.3en/byte) Dec=%.6fs (%.3en/byte)\n",
		algo, payloadB, symbolSize, parity, meanEnc, tByteEnc*1e9, meanDec, tByteDec*1e9)

	return Result{Algo: algo, PayloadBytes: payloadB, SymbolSize: symbolSize, Parity: parity,
		MeanEncSec: meanEnc, MeanDecSec: meanDec, EncPerByte: tByteEnc, DecPerByte: tByteDec}
}

// ---------------- main ----------------

func main() {
	runtime.GOMAXPROCS(1)
	results := []Result{}

	payloadSizes := []int{256 * 1024, 1024 * 1024}
	symbolSizes := []int{512, 1024}
	parities := []int{16, 64}
	iters := 5

	for _, B := range payloadSizes {
		for _, S := range symbolSizes {
			for _, P := range parities {
				results = append(results,
					bench("XOR", EncodeXOR, DecodeXOR, B, S, P, iters))
				results = append(results,
					bench("ReedSolomon", EncodeRS, DecodeRS, B, S, P, iters))
				results = append(results,
					bench("RaptorQ", EncodeRaptorQ, func(sh [][]byte, _ int) error {
						totalSymbols := int(math.Ceil(float64(B) / float64(S)))
						return DecodeRaptorQ(sh, S, totalSymbols)
					}, B, S, P, iters))
			}
		}
	}

	writeCSV("results.csv", results)
	printAlgorithmAverages(results)
}

// ---------------- Helpers ----------------

func writeCSV(filename string, results []Result) {
	f, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating CSV:", err)
		return
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	writer.Write([]string{"Algorithm", "PayloadBytes", "SymbolSize", "Parity", "MeanEncSec",
		"MeanDecSec", "EncPerByte(s)", "DecPerByte(s)"})

	for _, r := range results {
		writer.Write([]string{
			r.Algo,
			fmt.Sprintf("%d", r.PayloadBytes),
			fmt.Sprintf("%d", r.SymbolSize),
			fmt.Sprintf("%d", r.Parity),
			fmt.Sprintf("%.9f", r.MeanEncSec),
			fmt.Sprintf("%.9f", r.MeanDecSec),
			fmt.Sprintf("%.9e", r.EncPerByte),
			fmt.Sprintf("%.9e", r.DecPerByte),
		})
	}
	fmt.Println("\n✅ Results written to", filename)
}

func printAlgorithmAverages(results []Result) {
	sumsEnc := map[string]float64{}
	sumsDec := map[string]float64{}
	counts := map[string]int{}

	for _, r := range results {
		sumsEnc[r.Algo] += r.EncPerByte
		sumsDec[r.Algo] += r.DecPerByte
		counts[r.Algo]++
	}

	fmt.Println("\n========= Average Per-Byte Times (Aggregated) =========")
	for algo := range sumsEnc {
		meanEnc := sumsEnc[algo] / float64(counts[algo])
		meanDec := sumsDec[algo] / float64(counts[algo])
		fmt.Printf("%-12s | Enc=%.3e s/byte (%.3f ns/byte) | Dec=%.3e s/byte (%.3f ns/byte)\n",
			algo, meanEnc, meanEnc*1e9, meanDec, meanDec*1e9)
	}
	fmt.Println("=======================================================")
}
