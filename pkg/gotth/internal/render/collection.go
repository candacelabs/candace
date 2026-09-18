package render

import (
	"context"
	"fmt"
	"hash/maphash"
	"maps"
	"runtime/debug"
	"slices"
	"strings"
)

const childrenFailureSite = "children"

// collectionCache contains only identities and hashes, never retained markup
// or application state. It describes the last successfully sent collection.
type collectionCache struct {
	ids           []string
	hashes        map[string]uint64
	parentCurrent bool
}

type collectionResult struct {
	index int
	cache collectionCache
	whole bool
	dirty []string
}

// KnownID is called on the session actor, after pending sends and their commits
// finish. Ingress only admits static IDs or declared child namespaces; this
// exact check occurs before a browser event reaches application reduction.
func (v *Renderer) KnownID(id string) bool {
	_, known := v.known[id]
	return known
}

func (v *Renderer) publishIDs() {
	known := make(map[string]struct{}, len(v.reg.frags))
	for _, fragment := range v.reg.frags {
		known[fragment.ID] = struct{}{}
	}
	for _, collection := range v.collections {
		for _, id := range collection.ids {
			known[id] = struct{}{}
		}
	}
	v.known = known
}

func collectionChildren(parent Fragment, state any) (children []Fragment, failure *Failure) {
	defer func() {
		if value := recover(); value != nil {
			children = nil
			failure = &Failure{FragmentID: parent.ID, Site: childrenFailureSite, Value: value, Stack: debug.Stack()}
		}
	}()
	children = parent.Children(state)
	seen := make(map[string]bool, len(children))
	for _, child := range children {
		if err := validFragmentID(child.ID); err != nil {
			return nil, &Failure{FragmentID: parent.ID, Site: childrenFailureSite, Value: err}
		}
		if !strings.HasPrefix(child.ID, parent.ID+":") || child.ID == parent.ID+":" || seen[child.ID] || child.Render == nil || child.Children != nil {
			return nil, &Failure{FragmentID: parent.ID, Site: childrenFailureSite, Value: fmt.Errorf(
				"gotth-live: child %q of %q must have a unique ID inside the parent's colon namespace, a Render, and no Children: correct the collection projection", child.ID, parent.ID)}
		}
		seen[child.ID] = true
	}
	return children, nil
}

func childIDs(children []Fragment) []string {
	ids := make([]string, len(children))
	for index, child := range children {
		ids[index] = child.ID
	}
	return ids
}

func (v *Renderer) markCollection(index int, parent Fragment, previous, next any) []Failure {
	children, failure := collectionChildren(parent, next)
	if failure != nil {
		return []Failure{*failure}
	}
	var failures []Failure
	structural := parent.Dirty == nil
	if parent.Dirty != nil {
		structural, failure = callDirty(parent, previous, next)
		if failure != nil {
			failures = append(failures, *failure)
		}
	}
	if structural || !slices.Equal(childIDs(children), v.collections[index].ids) {
		v.parentDirty.set(index)
		v.dirty.set(index)
	}
	if v.parentDirty.has(index) {
		// A pending structural render already includes every current member.
		// Do not retain departed child IDs while backpressure coalesces changes.
		delete(v.childDirty, index)
		return failures
	}
	if v.childDirty[index] == nil {
		v.childDirty[index] = make(map[string]bool)
	}
	for _, child := range children {
		changed := child.Dirty == nil
		if child.Dirty != nil {
			changed, failure = callDirty(child, previous, next)
			if failure != nil {
				failures = append(failures, *failure)
			}
		}
		if changed {
			v.childDirty[index][child.ID] = true
			v.dirty.set(index)
		}
	}
	return failures
}

func (v *Renderer) collectionMarkup(ctx context.Context, state any, fragment Fragment, previousHash uint64, compare bool) (string, uint64, *Failure) {
	fctx := ctx
	var done func(suppressed, failed bool)
	if v.observe != nil {
		fctx, done = v.observe(ctx, fragment.ID)
	}
	v.buf.Reset()
	failure := v.callRender(fragment, fctx, state)
	hash := maphash.Bytes(v.reg.seed, v.buf.Bytes())
	if done != nil {
		done(failure == nil && compare && hash == previousHash, failure != nil)
	}
	if failure != nil {
		return "", 0, failure
	}
	return v.buf.String(), hash, nil
}

func (v *Renderer) renderCollection(ctx context.Context, state any, index int, parent Fragment, all bool, result *Result) {
	children, failure := collectionChildren(parent, state)
	if failure != nil {
		result.Failed = append(result.Failed, *failure)
		return
	}
	ids := childIDs(children)
	previous, initialized := v.collections[index]
	whole := all || !initialized || v.parentDirty.has(index) || !slices.Equal(previous.ids, ids)
	v.parentDirty.clear(index)
	pending := v.childDirty[index]
	delete(v.childDirty, index)
	cache := collectionCache{ids: ids, hashes: maps.Clone(previous.hashes), parentCurrent: whole}
	if whole {
		cache.hashes = make(map[string]uint64, len(children))
	}
	var updates []Update
	for _, child := range children {
		if !whole && !pending[child.ID] {
			continue
		}
		markup, hash, failed := v.collectionMarkup(ctx, state, child, previous.hashes[child.ID], !whole)
		if failed != nil {
			result.Failed = append(result.Failed, *failed)
			if whole {
				return
			}
			continue
		}
		cache.hashes[child.ID] = hash
		if !whole {
			if hash == previous.hashes[child.ID] {
				result.Suppressed = append(result.Suppressed, child.ID)
			} else {
				updates = append(updates, Update{FragmentID: child.ID, Op: OpMorph, HTML: markup})
			}
		}
	}
	if whole {
		markup, hash, failed := v.collectionMarkup(ctx, state, parent, v.hashes[index], !all && initialized && previous.parentCurrent && slices.Equal(previous.ids, ids))
		if failed != nil {
			result.Failed = append(result.Failed, *failed)
			return
		}
		if !all && initialized && previous.parentCurrent && hash == v.hashes[index] && slices.Equal(previous.ids, ids) {
			result.Suppressed = append(result.Suppressed, parent.ID)
			return
		}
		result.updated = append(result.updated, index)
		result.hashes = append(result.hashes, hash)
		updates = []Update{{FragmentID: parent.ID, Op: OpMorph, HTML: markup}}
	}
	if len(updates) != 0 {
		result.Updates = append(result.Updates, updates...)
		dirty := make([]string, len(updates))
		for index, update := range updates {
			dirty[index] = update.FragmentID
		}
		result.collections = append(result.collections, collectionResult{index: index, cache: cache, whole: whole, dirty: dirty})
	}
}
