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

//
// ====================================================
// Core FEC Implementations
// ====================================================
//

// ---------- XOR ----------
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

// ---------- Reed–Solomon ----------
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

	if err := enc.Encode(shards); err != nil {
		return nil, err
	}
	return shards, nil
}

func DecodeRS(shards [][]byte, parity int) error {
	total := len(shards)
	dataShards := total - parity
	if dataShards <= 0 {
		return fmt.Errorf("invalid shard config: data=%d parity=%d", dataShards, parity)
	}

	dec, err := rs.New(dataShards, parity)
	if err != nil {
		return err
	}

	// shards[0] = nil // simulate one missing data shard

	ok, err := dec.Verify(shards)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return dec.Reconstruct(shards)
}

// ---------- RaptorQ ----------
func EncodeRaptorQ(payload []byte, symbolSize, parity int) ([][]byte, error) {
	r := rq.NewRaptorQ(uint32(symbolSize))
	enc, err := r.CreateEncoder(payload)
	if err != nil {
		return nil, err
	}

	K := int(enc.BaseSymbolsNum())
	totalSymbols := K + parity
	out := make([][]byte, totalSymbols)
	for i := 0; i < totalSymbols; i++ {
		out[i] = enc.GenSymbol(uint32(i))
	}
	return out, nil
}

func DecodeRaptorQ(encoded [][]byte, symbolSize int, dataSizeBytes int) error {
	r := rq.NewRaptorQ(uint32(symbolSize))
	dec, err := r.CreateDecoder(uint32(dataSizeBytes))
	if err != nil {
		return err
	}

	for i, sym := range encoded {
		done, err := dec.AddSymbol(uint32(i), sym)
		if err != nil {
			return err
		}
		if done {
			break
		}
	}

	_, _, err = dec.Decode()
	return err
}

//
// ====================================================
// Benchmarking Framework
// ====================================================
//

type Result struct {
	Algo       string
	N          int
	S          int
	R          float64
	Parity     int
	PayloadB   int
	EncSec     float64
	DecSec     float64
	EncPerByte float64
	DecPerByte float64
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

func bench(algo string,
	encode func([]byte, int, int) ([][]byte, error),
	decode func([][]byte, int) error,
	N, S int, R float64, iters int) Result {

	payloadB := N * S
	parity := int(math.Ceil(float64(N) * R / 100.0))

	payload := make([]byte, payloadB)
	_, _ = rand.Read(payload)

	var encTimes, decTimes []float64
	for i := 0; i < iters; i++ {
		start := time.Now()
		shards, err := encode(payload, S, parity)
		if err != nil {
			fmt.Printf("%s encode error: %v\n", algo, err)
			return Result{}
		}
		encTimes = append(encTimes, time.Since(start).Seconds())

		if decode != nil {
			start = time.Now()
			if err := decode(shards, parity); err != nil {
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

	fmt.Printf("%-12s | N=%4d S=%5d R=%4.1f%% | Enc=%.6fs (%.3f ns/B) | Dec=%.6fs (%.3f ns/B)\n",
		algo, N, S, R, meanEnc, tByteEnc*1e9, meanDec, tByteDec*1e9)

	return Result{
		Algo:       algo,
		N:          N,
		S:          S,
		R:          R,
		Parity:     parity,
		PayloadB:   payloadB,
		EncSec:     meanEnc,
		DecSec:     meanDec,
		EncPerByte: tByteEnc,
		DecPerByte: tByteDec,
	}
}

//
// ====================================================
// Main loop (N, S, R grid)
// ====================================================
//

func main() {
	runtime.GOMAXPROCS(1)
	results := []Result{}

	Ns := []int{10, 20, 30, 40, 50}     // number of source symbols
	Ss := []int{64, 92, 120, 250, 512}  // bytes per symbol
	Rs := []float64{10, 20, 30, 40, 50} // redundancy %
	iters := 3

	for _, N := range Ns {
		for _, S := range Ss {
			for _, R := range Rs {
				xorDecode := func(sh [][]byte, _ int) error { return DecodeXOR(sh, []int{0}) }
				rsDecode := func(sh [][]byte, _ int) error { return DecodeRS(sh, int(math.Ceil(float64(N)*R/100.0))) }
				rqDecode := func(sh [][]byte, _ int) error { return DecodeRaptorQ(sh, S, N*S) }

				results = append(results, bench("XOR", EncodeXOR, xorDecode, N, S, R, iters))
				results = append(results, bench("ReedSolomon", EncodeRS, rsDecode, N, S, R, iters))
				results = append(results, bench("RaptorQ", EncodeRaptorQ, rqDecode, N, S, R, iters))
			}
		}
	}

	writeCSV("results_nsr.csv", results)
	printAverages(results)
}

//
// ====================================================
// Helpers
// ====================================================
//

func writeCSV(filename string, results []Result) {
	f, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating CSV:", err)
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{"Algorithm", "N", "S", "R%", "Parity", "PayloadBytes",
		"MeanEncSec", "MeanDecSec", "EncPerByte(s)", "DecPerByte(s)"})

	for _, r := range results {
		w.Write([]string{
			r.Algo,
			fmt.Sprintf("%d", r.N),
			fmt.Sprintf("%d", r.S),
			fmt.Sprintf("%.1f", r.R),
			fmt.Sprintf("%d", r.Parity),
			fmt.Sprintf("%d", r.PayloadB),
			fmt.Sprintf("%.9f", r.EncSec),
			fmt.Sprintf("%.9f", r.DecSec),
			fmt.Sprintf("%.9e", r.EncPerByte),
			fmt.Sprintf("%.9e", r.DecPerByte),
		})
	}
	fmt.Println("\n✅ Results written to", filename)
}

func printAverages(results []Result) {
	sumEnc := map[string]float64{}
	sumDec := map[string]float64{}
	count := map[string]int{}

	for _, r := range results {
		sumEnc[r.Algo] += r.EncPerByte
		sumDec[r.Algo] += r.DecPerByte
		count[r.Algo]++
	}

	fmt.Println("\n========= Average Per-Byte Encoding/Decoding =========")
	for algo := range sumEnc {
		avgEnc := sumEnc[algo] / float64(count[algo])
		avgDec := sumDec[algo] / float64(count[algo])
		fmt.Printf("%-12s | Enc=%.3e s/B (%.3f ns/B) | Dec=%.3e s/B (%.3f ns/B)\n",
			algo, avgEnc, avgEnc*1e9, avgDec, avgDec*1e9)
	}
	fmt.Println("=====================================================")
}
