package graphtool

import (
	"context"
	"fmt"
	"reflect"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const graphToolCheckPointID = "graph_tool_checkpoint_id"

type Compilable[I, O any] interface {
	Compile(ctx context.Context, opts ...compose.GraphCompileOption) (compose.Runnable[I, O], error)
}

type InvokableGraphTool[I, O any] struct {
	compilable     Compilable[I, O]
	compileOptions []compose.GraphCompileOption
	tInfo          *schema.ToolInfo
}

func NewInvokableGraphTool[I, O any](compilable Compilable[I, O], name, desc string, opts ...compose.GraphCompileOption) (*InvokableGraphTool[I, O], error) {
	tInfo, err := utils.GoStruct2ToolInfo[I](name, desc)
	if err != nil {
		return nil, err
	}
	return &InvokableGraphTool[I, O]{
		compilable:     compilable,
		compileOptions: opts,
		tInfo:          tInfo,
	}, nil
}

func (g *InvokableGraphTool[I, O]) Info(_ context.Context) (*schema.ToolInfo, error) {
	return g.tInfo, nil
}

type graphToolInterruptState struct {
	Data      []byte
	ToolInput string
}

func init() {
	schema.RegisterName[*graphToolInterruptState]("_papersilm_graph_tool_interrupt_state")
}

func (g *InvokableGraphTool[I, O]) InvokableRun(ctx context.Context, input string, opts ...tool.Option) (string, error) {
	var (
		checkpointStore *graphToolStore
		inputParams     I
		runnable        compose.Runnable[I, O]
		err             error
	)

	callOpts := []compose.Option{compose.WithCheckPointID(graphToolCheckPointID)}
	callOpts = append(callOpts, tool.GetImplSpecificOptions(&graphToolOptions{}, opts...).composeOpts...)

	wasInterrupted, hasState, state := tool.GetInterruptState[*graphToolInterruptState](ctx)
	compileOptions := append([]compose.GraphCompileOption{}, g.compileOptions...)
	if wasInterrupted && hasState {
		input = state.ToolInput
		checkpointStore = newResumeStore(state.Data)
	} else {
		checkpointStore = newEmptyStore()
	}
	compileOptions = append(compileOptions, compose.WithCheckPointStore(checkpointStore))

	if runnable, err = g.compilable.Compile(ctx, compileOptions...); err != nil {
		return "", err
	}

	inputParams = NewInstance[I]()
	if err := sonic.UnmarshalString(input, &inputParams); err != nil {
		return "", err
	}

	output, err := runnable.Invoke(ctx, inputParams, callOpts...)
	if err != nil {
		if _, ok := compose.ExtractInterruptInfo(err); !ok {
			return "", err
		}
		data, existed, getErr := checkpointStore.Get(ctx, graphToolCheckPointID)
		if getErr != nil {
			return "", getErr
		}
		if !existed {
			return "", fmt.Errorf("graph tool interrupt without checkpoint data")
		}
		return "", tool.CompositeInterrupt(ctx, "graph tool interrupt", &graphToolInterruptState{
			Data:      data,
			ToolInput: input,
		}, err)
	}

	return sonic.MarshalString(output)
}

type graphToolOptions struct {
	composeOpts []compose.Option
}

func WithGraphToolOption(opts ...compose.Option) tool.Option {
	return tool.WrapImplSpecificOptFn(func(opt *graphToolOptions) {
		opt.composeOpts = opts
	})
}

type graphToolStore struct {
	Data  []byte
	Valid bool
}

func newEmptyStore() *graphToolStore {
	return &graphToolStore{}
}

func newResumeStore(data []byte) *graphToolStore {
	return &graphToolStore{Data: data, Valid: true}
}

func (m *graphToolStore) Get(_ context.Context, _ string) ([]byte, bool, error) {
	if m.Valid {
		return m.Data, true, nil
	}
	return nil, false, nil
}

func (m *graphToolStore) Set(_ context.Context, _ string, checkPoint []byte) error {
	m.Data = checkPoint
	m.Valid = true
	return nil
}

func NewInstance[T any]() T {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	switch typ.Kind() {
	case reflect.Map:
		return reflect.MakeMap(typ).Interface().(T)
	case reflect.Slice, reflect.Array:
		return reflect.MakeSlice(typ, 0, 0).Interface().(T)
	case reflect.Ptr:
		elem := typ.Elem()
		origin := reflect.New(elem)
		inst := origin
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
			inst = inst.Elem()
			inst.Set(reflect.New(elem))
		}
		return origin.Interface().(T)
	default:
		var zero T
		return zero
	}
}
