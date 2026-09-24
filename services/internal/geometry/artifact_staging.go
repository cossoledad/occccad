package geometry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ArtifactStaging bridges immutable storage to OCCT's file-based evaluator.
// API/Jobs and their managed workers share this scratch directory, not durable
// artifact ownership. Each RPC owns its input copies, including concurrent RPCs.
func ArtifactStaging(store artifact.Store, local *artifact.LocalStore) grpc.DialOption {
	return grpc.WithChainUnaryInterceptor(artifactStagingInterceptor(store, local))
}

func artifactStagingInterceptor(store artifact.Store, local *artifact.LocalStore) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		switch req.(type) {
		case *workerv1.GetTopologyRequest, *workerv1.EvaluatePartRequest, *workerv1.InspectExchangeRequest, *workerv1.ImportExchangeRequest, *workerv1.ExportExchangeRequest:
		default:
			return invoke(ctx, method, req, reply, cc, opts...)
		}
		if store.Backend() == "LOCAL" {
			return invoke(ctx, method, req, reply, cc, opts...)
		}
		message, ok := req.(proto.Message)
		if !ok {
			return invoke(ctx, method, req, reply, cc, opts...)
		}
		clone := proto.Clone(message)
		var directories []string
		materialized := map[string]string{}
		defer func() {
			for _, dir := range directories {
				_ = os.RemoveAll(dir)
			}
		}()
		var visit func(protoreflect.Message) error
		visit = func(m protoreflect.Message) error {
			if ref, ok := m.Interface().(*workerv1.ArtifactReference); ok {
				if ref.GetObjectKey() == "" || ref.GetBackend() == "LOCAL" {
					return nil
				}
				if ref.GetBackend() != store.Backend() {
					return fmt.Errorf("unconfigured worker artifact backend %q", ref.GetBackend())
				}

				identity := fmt.Sprintf("%s:%s:%d", ref.GetObjectKey(), ref.GetSha256(), ref.GetSizeBytes())
				if key, ok := materialized[identity]; ok {
					ref.Backend = "LOCAL"
					ref.ObjectKey = key
					return nil
				}
				if ref.GetSizeBytes() > math.MaxInt64-1 {
					return fmt.Errorf("artifact size overflow")
				}
				reader, err := store.Open(ctx, ref.GetObjectKey())
				if err != nil {
					return err
				}
				defer reader.Close()
				root := filepath.Join(local.Root(), "exchange", "inputs")
				if err := os.MkdirAll(root, 0700); err != nil {
					return err
				}
				dir, err := os.MkdirTemp(root, "rpc-*")
				if err != nil {
					return err
				}
				directories = append(directories, dir)
				file, err := os.Create(filepath.Join(dir, "input"))
				if err != nil {
					return err
				}
				hash := sha256.New()
				size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, int64(ref.GetSizeBytes())+1))
				closeErr := file.Close()
				if copyErr != nil {
					return copyErr
				}
				if closeErr != nil {
					return closeErr
				}
				if size != int64(ref.GetSizeBytes()) || hex.EncodeToString(hash.Sum(nil)) != ref.GetSha256() {
					return fmt.Errorf("worker artifact integrity mismatch")
				}
				key, err := filepath.Rel(local.Root(), file.Name())
				if err != nil {
					return err
				}
				ref.Backend = "LOCAL"
				ref.ObjectKey = filepath.ToSlash(key)
				materialized[identity] = ref.ObjectKey
				return nil
			}
			var result error
			m.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
				if field.Message() == nil {
					return true
				}
				if field.IsList() {
					list := value.List()
					for i := 0; i < list.Len(); i++ {
						if result = visit(list.Get(i).Message()); result != nil {
							return false
						}
					}
				} else if !field.IsMap() {
					result = visit(value.Message())
				}
				return result == nil
			})
			return result
		}
		if err := visit(clone.ProtoReflect()); err != nil {
			return err
		}
		return invoke(ctx, method, clone, reply, cc, opts...)
	}
}
