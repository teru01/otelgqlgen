// Copyright Ravil Galaktionov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otelgqlgen

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.opentelemetry.io/otel/attribute"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const (
	testQueryName     = "NamedQuery"
	namelessQueryName = "nameless-operation"
	testComplexity    = 5
)

func TestChildSpanFromGlobalTracer(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestExecutedOperationNameAsSpanNameWithOperationNameParameter(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())

	query := url.QueryEscape("query A {__typename} query B {__typename} query C {__typename}")
	r := httptest.NewRequest("GET", fmt.Sprintf("/foo?operationName=C&query=%s", query), nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, "C", codes.Ok, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestExecutedOperationNameAsSpanNameWithoutOperationNameParameter(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())

	query := url.QueryEscape("query ThisIsOperationName {__typename}")
	r := httptest.NewRequest("GET", fmt.Sprintf("/foo?query=%s", query), nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, "ThisIsOperationName", codes.Ok, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestChildSpanFromGlobalTracerWithNamed(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())

	body := strings.NewReader(fmt.Sprintf("{\"operationName\":\"%s\",\"variables\":{},\"query\":\"query %s {\\n  name\\n}\\n\"}", testQueryName, testQueryName))
	r := httptest.NewRequest("POST", "/foo", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, testQueryName, codes.Ok, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestChildSpanFromCustomTracer(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware(WithTracerProvider(provider)))

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestChildSpanWithComplexityExtension(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware(WithComplexityExtensionName("APQ")))

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestChildSpanWithDropFromFields(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware(WithCreateSpanFromFields(func(ctx *graphql.FieldContext) bool {
		return ctx.IsResolver || ctx.IsMethod
	})))

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	spans := spanRecorder.Ended()
	if got, expected := len(spans), 1; got != expected {
		t.Fatalf("got %d spans, expected %d", got, expected)
	}

	responseSpan := spans[0]
	if !responseSpan.SpanContext().IsValid() {
		t.Fatalf("invalid span created: %#v", responseSpan.SpanContext())
	}

	if responseSpan.Name() != namelessQueryName {
		t.Errorf("expected name on span %s; got: %q", namelessQueryName, responseSpan.Name())
	}

	for _, s := range spanRecorder.Ended() {
		assert.Equal(t, s.Status().Code, codes.Ok)
	}

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestGetSpanNotInstrumented(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if span.IsValid() {
			t.Fatalf("unexpected span: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestChildSpanFromGlobalTracerWithError(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServerError(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())
	var gqlErrors gqlerror.List
	var respErrors gqlerror.List
	srv.AroundResponses(func(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
		resp := next(ctx)
		gqlErrors = graphql.GetErrors(ctx)
		respErrors = resp.Errors
		return resp
	})

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Error, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, len(gqlErrors))
	assert.Equal(t, gqlErrors, respErrors)
}

func TestChildSpanFromGlobalTracerWithComplexity(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())
	srv.Use(extension.FixedComplexityLimit(testComplexity))

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	// second span because it's response span where stored RequestComplexityLimit attribute
	attributes := spanRecorder.Ended()[1].Attributes()
	var found bool
	for _, a := range attributes {
		if a.Key == ("gql.request.complexityLimit") {
			found = true
			assert.Equal(t, int(a.Value.AsInt64()), testComplexity)
		}
	}

	assert.True(t, found)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestOperationName(t *testing.T) {
	operation := "ExampleOperationName"
	ctx := SetOperationName(context.Background(), operation)
	assert.Equal(t, operation, GetOperationName(ctx))
}

func TestOperationNameInvalidInputJSON(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(_ context.Context) (interface{}, error) {
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())

	// invalid json body
	body := strings.NewReader("")
	r := httptest.NewRequest("POST", "/foo", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.JSONEq(t, `{"errors":[{"message":"json request body could not be decoded: EOF body:"}],"data":null}`, w.Body.String())
}

func TestVariablesAttributes(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware())

	body := strings.NewReader("{\"variables\":{\"id\":1},\"query\":\"query ($id: Int!) {\\n  find(id: $id)\\n}\\n\"}")
	r := httptest.NewRequest("POST", "/foo", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	spans := spanRecorder.Ended()
	assert.Len(t, spans[1].Attributes(), 2)
	assert.Equal(t, attribute.Key("gql.request.query"), spans[1].Attributes()[0].Key)
	assert.Equal(t, attribute.Key("gql.request.variables.id"), spans[1].Attributes()[1].Key)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestVariablesAttributesCustomBuilder(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware(WithRequestVariablesAttributesBuilder(func(requestVariables map[string]interface{}) []attribute.KeyValue {
		variables := make([]attribute.KeyValue, 0, len(requestVariables))
		for k, v := range requestVariables {
			variables = append(variables,
				attribute.String(k, fmt.Sprintf("%+v", v)),
			)
		}
		return variables
	})))

	body := strings.NewReader("{\"variables\":{\"id\":1},\"query\":\"query ($id: Int!) {\\n  find(id: $id)\\n}\\n\"}")
	r := httptest.NewRequest("POST", "/foo", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	spans := spanRecorder.Ended()
	assert.Len(t, spans[1].Attributes(), 2)
	assert.Equal(t, attribute.Key("gql.request.query"), spans[1].Attributes()[0].Key)
	assert.Equal(t, attribute.Key("id"), spans[1].Attributes()[1].Key)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestVariablesAttributesDisabled(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware(WithoutVariables()))

	body := strings.NewReader("{\"variables\":{\"id\":1},\"query\":\"query ($id: Int!) {\\n  find(id: $id)\\n}\\n\"}")
	r := httptest.NewRequest("POST", "/foo", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	spans := spanRecorder.Ended()
	assert.Len(t, spans[1].Attributes(), 1)
	assert.Equal(t, attribute.Key("gql.request.query"), spans[1].Attributes()[0].Key)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestNilResponse(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return (*graphql.Response)(nil), nil
	})
	srv.Use(Middleware())

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindServer)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestWithSpanKindSelector(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	srv := newMockServer(func(ctx context.Context) (interface{}, error) {
		span := trace.SpanContextFromContext(ctx)
		if !span.IsValid() {
			t.Fatalf("invalid span wrapping handler: %#v", span)
		}
		return &graphql.Response{Data: []byte(`{"name":"test"}`)}, nil
	})
	srv.Use(Middleware(WithSpanKindSelector(func(operationName string) trace.SpanKind {
		if operationName == "nameless-operation" || operationName == "name/name" {
			return trace.SpanKindClient
		}
		return trace.SpanKindServer
	})))

	r := httptest.NewRequest("GET", "/foo?query={name}", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, r)

	testSpans(t, spanRecorder, namelessQueryName, codes.Ok, trace.SpanKindClient)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// newMockServer provides a server for use in resolver tests that isn't relying on generated code.
// It isn't a perfect reproduction of a generated server, but it aims to be good enough to
// test the handler package without relying on codegen.
func newMockServer(resolver func(ctx context.Context) (interface{}, error)) *handler.Server {
	schema := gqlparser.MustLoadSchema(&ast.Source{Input: `
		type Query {
			name: String!
			find(id: Int!): String!
		}
		type Mutation {
			name: String!
		}
		type Subscription {
			name: String!
		}
	`})
	srv := handler.New(&graphql.ExecutableSchemaMock{
		ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
			rc := graphql.GetOperationContext(ctx)
			switch rc.Operation.Operation {
			case ast.Query:
				ran := false
				return func(ctx context.Context) *graphql.Response {
					if ran {
						return nil
					}
					ran = true
					// Field execution happens inside the generated code, lets simulate some of it.
					ctx = graphql.WithFieldContext(ctx, &graphql.FieldContext{
						Object: "Query",
						Field: graphql.CollectedField{
							Field: &ast.Field{
								Name:       "name",
								Alias:      "alias",
								Definition: schema.Types["Query"].Fields.ForName("name"),
								ObjectDefinition: &ast.Definition{
									Kind:        "kind",
									Description: "description",
									Name:        "name",
								},
							},
						},
					})
					res, err := graphql.GetOperationContext(ctx).ResolverMiddleware(ctx, resolver)
					if err != nil {
						panic(err)
					}
					return res.(*graphql.Response)
				}
			default:
				return graphql.OneShot(graphql.ErrorResponse(ctx, "unsupported GraphQL operation"))
			}
		},
		SchemaFunc: func() *ast.Schema {
			return schema
		},
		ComplexityFunc: func(_ context.Context, _ string, _ string, childComplexity int, _ map[string]any) (int, bool) {
			return childComplexity, true
		},
	})
	srv.AddTransport(&transport.GET{})
	srv.AddTransport(&transport.POST{})

	return srv
}

// newMockServerError provides a server for use in resolver error tests that isn't relying on generated code.
// It isn't a perfect reproduction of a generated server, but it aims to be good enough to
// test the handler package without relying on codegen.
func newMockServerError(resolver func(ctx context.Context) (interface{}, error)) *handler.Server {
	schema := gqlparser.MustLoadSchema(&ast.Source{Input: `
		type Query {
			name: String!
		}
	`})
	srv := handler.New(&graphql.ExecutableSchemaMock{
		ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
			rc := graphql.GetOperationContext(ctx)
			switch rc.Operation.Operation {
			case ast.Query:
				ran := false
				return func(ctx context.Context) *graphql.Response {
					if ran {
						return nil
					}
					ran = true
					// Field execution happens inside the generated code, lets simulate some of it.
					ctx = graphql.WithFieldContext(ctx, &graphql.FieldContext{
						Object: "Query",
						Field: graphql.CollectedField{
							Field: &ast.Field{
								Name:       "name",
								Alias:      "alias",
								Definition: schema.Types["Query"].Fields.ForName("name"),
								ObjectDefinition: &ast.Definition{
									Kind:        "kind",
									Description: "description",
									Name:        "name",
								},
							},
						},
					})
					graphql.AddError(ctx, fmt.Errorf("resolver error"))

					res, err := graphql.GetOperationContext(ctx).ResolverMiddleware(ctx, resolver)
					if err != nil {
						panic(err)
					}
					return res.(*graphql.Response)
				}
			default:
				return graphql.OneShot(graphql.ErrorResponse(ctx, "unsupported GraphQL operation"))
			}
		},
		SchemaFunc: func() *ast.Schema {
			return schema
		},
	})
	srv.AddTransport(&transport.GET{})

	return srv
}

func testSpans(t *testing.T, spanRecorder *tracetest.SpanRecorder, spanName string, spanCode codes.Code, spanKind trace.SpanKind) {
	spans := spanRecorder.Ended()
	if got, expected := len(spans), 2; got != expected {
		t.Fatalf("got %d spans, expected %d", got, expected)
	}
	responseSpan := spans[1]
	if !responseSpan.SpanContext().IsValid() {
		t.Fatalf("invalid span created: %#v", responseSpan.SpanContext())
	}

	if responseSpan.Name() != spanName {
		t.Errorf("expected name on span %s; got: %q", spanName, responseSpan.Name())
	}

	for _, s := range spanRecorder.Ended() {
		assert.Equal(t, spanCode, s.Status().Code)
		assert.Equal(t, spanKind, s.SpanKind())
	}
}
