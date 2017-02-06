package resolve

import (
	"fmt"
	"reflect"

	"github.com/graphql-go/graphql/language/ast"
)

type listResolver struct {
	elemResolver Resolver
}

func (lr *listResolver) Execute(rc *resolutionContext, resolver reflect.Value) {
	qnode := rc.qnode
	// TODO: Maybe handle nil values?
	if resolver.IsNil() {
		return
	}

	count := resolver.Len()
	for i := 0; i < count; i++ {
		iv := resolver.Index(i)
		go lr.elemResolver.Execute(rc.Child(qnode), iv)
	}
}

type chanListResolver struct {
	*listResolver
}

func (fr *chanListResolver) Execute(rc *resolutionContext, resolver reflect.Value) {
	if resolver.IsNil() {
		return
	}
	go func() {
		done := rc.ctx.Done()
		doneVal := reflect.ValueOf(done)
		for {
			// resolver = <-chan string
			// select {
			chosen, recv, recvOk := reflect.Select([]reflect.SelectCase{
				// case rval := <-resolver:
				{
					Chan: resolver,
					Dir:  reflect.SelectRecv,
				},
				// case <-ctx.Done()
				{
					Chan: doneVal,
					Dir:  reflect.SelectRecv,
				},
			})
			switch chosen {
			case 0:
				if !recvOk {
					return
				}
				go fr.elemResolver.Execute(rc, recv)
				continue
			case 1:
				return
			}
		}
	}()
}

func (rt *ResolverTree) buildListResolver(pair TypeResolverPair, ldef *ast.List) (Resolver, error) {
	isChan := pair.ResolverType.Kind() == reflect.Chan
	if isChan {
		if pair.ResolverType.ChanDir() != reflect.RecvDir {
			return nil, fmt.Errorf("Invalid array type %s, (should be a %v, is a %v)", pair.ResolverType.String(), reflect.RecvDir, pair.ResolverType.ChanDir())
		}
	} else {
		if pair.ResolverType.Kind() != reflect.Slice {
			return nil, fmt.Errorf("Expected array type, got %v (should be a slice or a chan).", pair.ResolverType.String())
		}
	}

	// The type in the pair will be a []*ResolverType or []ResolverType or <-chan ResolverType etc...
	arrElem := pair.ResolverType.Elem()
	if arrElem.Kind() == reflect.Ptr {
		arrElem = arrElem.Elem()
	}

	// Follow list element
	elemResolver, err := rt.buildFollowResolver(arrElem, ldef.Type)
	if err != nil {
		return nil, err
	}

	res := &listResolver{
		elemResolver: elemResolver,
	}

	if isChan {
		cres := &chanListResolver{
			listResolver: res,
		}

		return cres, nil
	}

	return res, nil
}
