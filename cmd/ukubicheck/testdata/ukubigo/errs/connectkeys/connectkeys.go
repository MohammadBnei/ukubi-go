// Package connectkeys stubs ukubi-go's connectkeys.
package connectkeys

type Keys struct{ n int }

func New(...any) *Keys { return &Keys{} }

func (k *Keys) WithInterceptors(...any) any { return k.n }
