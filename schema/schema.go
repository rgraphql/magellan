package schema

import (
	"context"
	"errors"
	"reflect"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/rgraphql/magellan/qtree"
	"github.com/rgraphql/magellan/resolve"
	proto "github.com/rgraphql/rgraphql/pkg/proto"
)

type Schema struct {
	Document    *ast.Document
	Definitions *ASTParts

	queryResolver resolve.Resolver
	rootResolver  reflect.Value
}

func FromDocument(doc *ast.Document) *Schema {
	// Transform all named pointers -> actual pointers.
	definitions := DocumentToParts(doc)
	return &Schema{
		Document:    doc,
		Definitions: definitions,
	}
}

func Parse(docStr string) (*Schema, error) {
	doc, err := parser.Parse(
		parser.ParseParams{
			Source: docStr,
			Options: parser.ParseOptions{
				NoLocation: true,
				NoSource:   true,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return FromDocument(doc), nil
}

func (s *Schema) SetResolvers(rootQueryResolver interface{}) error {
	if s.Definitions == nil {
		return errors.New("Definitions have not been parsed yet.")
	}
	if len(s.Definitions.Schemas) == 0 {
		return errors.New("No schema block given in schema AST.")
	}

	rootQueryObj, ok := s.Definitions.RootQuery.(*ast.ObjectDefinition)
	if !ok || rootQueryObj.Name == nil {
		return errors.New("Root query schema not defined, or not an object.")
	}

	rootQueryResolverType := reflect.TypeOf(rootQueryResolver)
	rootPair := resolve.TypeResolverPair{GqlType: rootQueryObj, ResolverType: rootQueryResolverType}

	rt := resolve.NewResolverTree(s.Definitions)
	rr, err := rt.BuildResolver(rootPair)
	if err != nil {
		return err
	}

	s.queryResolver = rr
	s.rootResolver = reflect.ValueOf(rootQueryResolver)
	return nil
}

func (s *Schema) HasResolvers() bool {
	return s.queryResolver != nil && s.rootResolver.IsValid() && !s.rootResolver.IsNil()
}

type QueryExecution interface {
	// Return the message channel (singleton).
	Messages() <-chan *proto.RGQLServerMessage
	// Wait for all resolvers to finish executing.
	Wait()
	// Cancel the query execution.
	Cancel()
}

// Start execution of a query tree.
func (s *Schema) StartQuery(ctx context.Context, query *qtree.QueryTreeNode) QueryExecution {
	return resolve.StartQuery(s.queryResolver, ctx, s.rootResolver, query)
}

// BuildQueryTree builds a new query tree from this schema.
func (s *Schema) BuildQueryTree() (*qtree.QueryTreeNode, error) {
	if s.Definitions == nil || s.Definitions.RootQuery == nil {
		return nil, errors.New("Schema not parsed or root query object not found.")
	}
	return qtree.NewQueryTree(s.Definitions.RootQuery.(*ast.ObjectDefinition), s.Definitions), nil
}
