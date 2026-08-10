package labyrinth

import (
	"container/heap"

	"github.com/gryph/omnidex/internal/cognition"
)

type solverNode struct {
	facts   factSet
	cost    int
	actions []cognition.ActionRequest
	key     string
	index   int
}

type solverQueue []*solverNode

func (queue solverQueue) Len() int { return len(queue) }

func (queue solverQueue) Less(left, right int) bool {
	if queue[left].cost != queue[right].cost {
		return queue[left].cost < queue[right].cost
	}
	return queue[left].key < queue[right].key
}

func (queue solverQueue) Swap(left, right int) {
	queue[left], queue[right] = queue[right], queue[left]
	queue[left].index = left
	queue[right].index = right
}

func (queue *solverQueue) Push(value any) {
	node := value.(*solverNode)
	node.index = len(*queue)
	*queue = append(*queue, node)
}

func (queue *solverQueue) Pop() any {
	old := *queue
	node := old[len(old)-1]
	old[len(old)-1] = nil
	node.index = -1
	*queue = old[:len(old)-1]
	return node
}

func newSolverQueue(initial *solverNode) *solverQueue {
	queue := &solverQueue{initial}
	heap.Init(queue)
	return queue
}
