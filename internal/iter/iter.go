/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package iter

import (
	"iter"
)

// IterErr is an extension to the iter package that allows for returning errors.
type IterErr[T any] struct { //nolint:revive
	Iter  iter.Seq[T]
	Error error
}

func noSeq[T any](_ func(T) bool) {}

// Error returns an IterErr with the error preset and an empty iterator.
func Error[T any](err error) *IterErr[T] {
	return &IterErr[T]{
		Iter:  noSeq[T],
		Error: err,
	}
}

// ForEach calls the given callback for each member of the iterator, stopping on
// and returning the first error encountered.
func (i *IterErr[T]) ForEach(fn func(T) error) error {
	for item := range i.Iter {
		if err := fn(item); err != nil {
			return err
		}
	}

	return i.Error
}

type Scanner interface {
	Close() error
	Next() bool
	Scan(params ...any) error
	Err() error
}

// Rows creates an IterErr from a database Scanner (like sql.Rows).
func Rows[T any](rows Scanner, fn func(Scanner) (T, error)) *IterErr[T] {
	var ie IterErr[T]

	ie.Iter = func(yield func(T) bool) {
		defer rows.Close()

		for rows.Next() {
			v, err := fn(rows)
			if err != nil {
				ie.Error = err

				return
			}

			if !yield(v) {
				break
			}
		}

		ie.Error = rows.Err()
	}

	return &ie
}
