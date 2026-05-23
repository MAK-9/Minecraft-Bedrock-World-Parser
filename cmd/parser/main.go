package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mak-9/minecraft-bedrock-world-parser/pkg/export"
	"github.com/mak-9/minecraft-bedrock-world-parser/pkg/world"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "minecraft-bedrock-parser",
		Short: "Parse Minecraft Bedrock worlds into SQLite for dynamic maps",
	}
	root.AddCommand(parseCmd(), verifyCmd())
	return root
}

// --- parse command ---

type parseFlags struct {
	output      string
	dimension   int
	region      string
	surfaceOnly bool
	copyFirst   bool
	verbose     bool
}

func parseCmd() *cobra.Command {
	f := &parseFlags{}
	cmd := &cobra.Command{
		Use:   "parse <world-path>",
		Short: "Parse a Bedrock world and write to SQLite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runParse(args[0], f)
		},
	}
	cmd.Flags().StringVarP(&f.output, "output", "o", "", "Output SQLite file (default: <world-name>.db)")
	cmd.Flags().IntVarP(&f.dimension, "dimension", "d", -1, "Dimension: 0=Overworld, 1=Nether, 2=End, -1=all")
	cmd.Flags().StringVar(&f.region, "region", "", "Limit to region: x1,z1,x2,z2")
	cmd.Flags().BoolVar(&f.surfaceOnly, "surface-only", false, "Only export surface blocks")
	cmd.Flags().BoolVar(&f.copyFirst, "copy-first", false, "Copy world before opening (required for live servers)")
	cmd.Flags().BoolVarP(&f.verbose, "verbose", "v", false, "Verbose output")
	return cmd
}

func runParse(worldPath string, f *parseFlags) error {
	worldPath = filepath.Clean(worldPath)

	if f.copyFirst {
		tmpDir, err := os.MkdirTemp("", "mcparser-*")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		copyDst := filepath.Join(tmpDir, filepath.Base(worldPath))
		if f.verbose {
			fmt.Printf("Copying world to %s …\n", copyDst)
		}
		if err := world.CopyWorld(worldPath, copyDst); err != nil {
			return fmt.Errorf("copying world: %w", err)
		}
		worldPath = copyDst
	}

	wr, err := world.Open(worldPath)
	if err != nil {
		return fmt.Errorf("opening world: %w", err)
	}
	defer wr.Close()

	info := wr.Info
	if f.verbose {
		fmt.Printf("World: %q  Seed: %d  Spawn: (%d,%d,%d)  Mode: %s\n",
			info.LevelName, info.Seed, info.SpawnX, info.SpawnY, info.SpawnZ, info.GameTypeName())
	}

	outPath := f.output
	if outPath == "" {
		name := info.LevelName
		if name == "" {
			name = filepath.Base(worldPath)
		}
		outPath = name + ".db"
	}

	db, err := export.OpenDB(outPath)
	if err != nil {
		return fmt.Errorf("opening output DB: %w", err)
	}
	defer db.Close()

	if err := export.WriteWorldInfo(db, info); err != nil {
		return fmt.Errorf("writing world info: %w", err)
	}

	exporter, err := export.NewChunkExporter(db, f.surfaceOnly)
	if err != nil {
		return fmt.Errorf("creating exporter: %w", err)
	}

	var chunkCount, errCount int
	err = wr.IterateChunks(f.dimension, func(cd world.ChunkData) error {
		chunkCount++
		if f.verbose && chunkCount%500 == 0 {
			fmt.Printf("\r  Chunks processed: %d …", chunkCount)
		}
		if err := exporter.ExportChunk(cd); err != nil {
			errCount++
			if f.verbose {
				fmt.Fprintf(os.Stderr, "\nWarning: chunk (%d,%d): %v\n", cd.Key.X, cd.Key.Z, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("iterating chunks: %w", err)
	}

	if err := exporter.Close(); err != nil {
		return fmt.Errorf("finalizing export: %w", err)
	}

	fmt.Printf("\nDone. Chunks: %d, Errors: %d → %s\n", chunkCount, errCount, outPath)
	return nil
}

// --- verify command ---

func verifyCmd() *cobra.Command {
	var copyFirst bool
	cmd := &cobra.Command{
		Use:   "verify <world-path>",
		Short: "Validate a world by parsing it and reporting statistics (no output file)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(args[0], copyFirst)
		},
	}
	cmd.Flags().BoolVar(&copyFirst, "copy-first", false, "Copy world before opening (required for live servers)")
	return cmd
}

func runVerify(worldPath string, copyFirst bool) error {
	worldPath = filepath.Clean(worldPath)

	if copyFirst {
		tmpDir, err := os.MkdirTemp("", "mcparser-verify-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		dst := filepath.Join(tmpDir, filepath.Base(worldPath))
		if err := world.CopyWorld(worldPath, dst); err != nil {
			return err
		}
		worldPath = dst
	}

	wr, err := world.Open(worldPath)
	if err != nil {
		return fmt.Errorf("opening world: %w", err)
	}
	defer wr.Close()

	info := wr.Info
	fmt.Printf("World:   %q\n", info.LevelName)
	fmt.Printf("Seed:    %d\n", info.Seed)
	fmt.Printf("Spawn:   (%d, %d, %d)\n", info.SpawnX, info.SpawnY, info.SpawnZ)
	fmt.Printf("Mode:    %s\n", info.GameTypeName())

	var (
		chunkCounts   [3]int
		subChunkCount int
		subChunkErrs  int
		beCount       int
		beErrs        int
		entCount      int
		entErrs       int
		samples       []string
	)

	type dimKey struct{ dim int }
	_ = dimKey{}

	err = wr.IterateChunks(-1, func(cd world.ChunkData) error {
		dim := int(cd.Key.Dimension)
		if dim >= 0 && dim < 3 {
			chunkCounts[dim]++
		}
		subChunkCount += len(cd.SubChunks)
		beCount += len(cd.BlockEntities)
		entCount += len(cd.Entities)

		// Collect a few sample surface blocks from chunk (0,0) overworld
		if cd.Key.X == 0 && cd.Key.Z == 0 && cd.Key.Dimension == world.DimOverworld && len(samples) < 5 {
			for _, sb := range world.FindSurfaceBlocks(cd) {
				if len(samples) < 5 {
					samples = append(samples, fmt.Sprintf("  (%d,%d) y=%d %s", sb.X, sb.Z, sb.Y, sb.Block.Name))
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	totalChunks := chunkCounts[0] + chunkCounts[1] + chunkCounts[2]
	fmt.Printf("\nChunks found:    %d\n", totalChunks)
	fmt.Printf("  Overworld:     %d\n", chunkCounts[0])
	fmt.Printf("  Nether:        %d\n", chunkCounts[1])
	fmt.Printf("  End:           %d\n", chunkCounts[2])
	fmt.Printf("SubChunk decode errors: %d / %d\n", subChunkErrs, subChunkCount)
	fmt.Printf("Block entity decode errors: %d / %d\n", beErrs, beCount)
	fmt.Printf("Entity decode errors: %d / %d\n", entErrs, entCount)

	if len(samples) > 0 {
		fmt.Printf("\nSample surface blocks (chunk 0,0 overworld):\n")
		for _, s := range samples {
			fmt.Println(s)
		}
	}
	return nil
}
