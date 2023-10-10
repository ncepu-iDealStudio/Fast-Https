package cache

import "fmt"

// reference https://www.cnblogs.com/qxcheng/p/15505415.html

const RED = 0
const BLACK = 1

type Type int

type RBNode struct {
	color               uint8
	key                 Type
	RbCacheNode         *CacheNode
	left, right, parent *RBNode
}

type RBRoot struct {
	node *RBNode
}

func rb_is_red(node *RBNode) bool {
	return node.color == RED
}

func rb_is_black(node *RBNode) bool {
	return node.color == BLACK
}

func rb_set_red(node *RBNode) {
	node.color = RED
}

func rb_set_black(node *RBNode) {
	node.color = BLACK
}

// 前序遍历
func PreTraverse(p *RBNode) {
	if p != nil {
		fmt.Printf("%d ", p.key)
		PreTraverse(p.left)
		PreTraverse(p.right)
	}
}

// FindFirst node
func FindFirst(p *RBNode) *RBNode {
	var temp *RBNode
	temp = p
	for temp.left != nil {
		temp = temp.left
	}
	return temp
}

// 中序遍历
func InTraverse(p *RBNode) {
	if p != nil {
		InTraverse(p.left)
		fmt.Printf("%d ", p.key)
		InTraverse(p.right)
	}
}

// 前序遍历
func PostTraverse(p *RBNode) {
	if p != nil {
		PostTraverse(p.left)
		PostTraverse(p.right)
		fmt.Printf("%d ", p.key)
	}
}

// 查找键值为key的节点
func Find(node *RBNode, key Type) *RBNode {
	for node != nil && node.key != key {
		if key < node.key {
			node = node.left
		} else {
			node = node.right
		}
	}
	return node
}

// 打印红黑树
func Print(node *RBNode, key Type, direction int) {
	if node != nil {
		if direction == 0 {
			fmt.Printf("%2d(B) is root\n", node.key)
		} else {
			var color, _direction string
			if rb_is_red(node) {
				color = "R"
			} else {
				color = "B"
			}
			if direction == 1 {
				_direction = "right"
			} else {
				_direction = "left"
			}
			// TODO: 这里key 和 %d 的关系
			fmt.Printf("%2d(%s) is %2d's %6s child\n", node.key, color, key, _direction)
		}
		Print(node.left, node.key, -1)
		Print(node.right, node.key, 1)
	}
}

// 打印根节点的所有路径(检测两条红黑树特性)
func PrintRoute(node *RBNode) {
	if node == nil {
		return
	}
	if node.left == nil && node.right == nil {
		var tmp *RBNode = node
		var num int
		for tmp != nil {
			var color string
			if rb_is_red(tmp) {
				color = "R"
			} else {
				color = "B"
				num++
			}
			fmt.Printf("%2d(%s)-->", tmp.key, color)
			if tmp.parent != nil && (tmp.color == RED && tmp.parent.color == RED) {
				fmt.Println("检测到违反红黑树特性：红节点的子节点是红节点")
			}
			tmp = tmp.parent
		}
		fmt.Printf("共 %d 个黑节点\n", num)
	}
	PrintRoute(node.left)
	PrintRoute(node.right)
}

func left_rotate(root *RBRoot, x *RBNode) {
	var y *RBNode = x.right

	// ly 和 x 的关系
	x.right = y.left
	if y.left != nil {
		y.left.parent = x
	}

	// px 和 y 的关系（要考虑px为空，即x为根节点的情况）
	y.parent = x.parent
	if x.parent == nil {
		root.node = y
	} else {
		if x.parent.left == x {
			x.parent.left = y
		} else {
			x.parent.right = y
		}
	}

	// y 和 x 的关系
	y.left = x
	x.parent = y

}

func right_rotate(root *RBRoot, y *RBNode) {
	var x *RBNode = y.left

	// rx 和 y 的关系
	y.left = x.right
	if x.right != nil {
		x.right.parent = y
	}

	// py 和 x 的关系（要考虑py为空，即y为根节点的情况）
	x.parent = y.parent
	if y.parent == nil {
		root.node = x
	} else {
		if y.parent.right == y {
			y.parent.right = x
		} else {
			y.parent.left = x
		}
	}

	// y 和 x 的关系
	x.right = y
	y.parent = x
}

// 添加节点：将节点(node)插入到红黑树中
func AddInRbtree(root *RBRoot, node *RBNode) {
	var y *RBNode
	var x *RBNode = root.node

	// 找到node应插入位置的父节点
	for x != nil {
		y = x
		if node.key < x.key {
			x = x.left
		} else {
			x = x.right
		}
	}
	// 设置node和父节点的关系
	node.parent = y
	if y != nil {
		if node.key < y.key {
			y.left = node
		} else {
			y.right = node
		}
	} else {
		root.node = node
	}
	// 设置节点为红色
	node.color = RED
	// 修正为红黑树
	add_fixup(root, node)

}

// 红黑树插入修正
func add_fixup(root *RBRoot, node *RBNode) {
	var parent, gparent *RBNode

	// 若“父节点存在，并且父节点的颜色是红色”
	for parent = node.parent; parent != nil && rb_is_red(parent); {
		gparent = parent.parent

		//若“父节点”是“祖父节点的左孩子”
		if parent == gparent.left {
			var uncle *RBNode = gparent.right
			// Case 1条件：叔叔节点是红色
			if uncle != nil && rb_is_red(uncle) {
				rb_set_black(uncle)
				rb_set_black(parent)
				rb_set_red(gparent)
				node = gparent
				continue
			}
			// Case 2条件：叔叔是黑色，且当前节点是右孩子
			if parent.right == node {
				left_rotate(root, parent)
				var tmp *RBNode = parent
				parent = node
				node = tmp
			}
			// Case 3条件：叔叔是黑色，且当前节点是左孩子。
			rb_set_black(parent)
			rb_set_red(gparent)
			right_rotate(root, gparent)
		} else {
			//若“父节点”是“祖父节点的右孩子”

			var uncle *RBNode = gparent.left
			// Case 1条件：叔叔节点是红色
			if uncle != nil && rb_is_red(uncle) {
				rb_set_black(uncle)
				rb_set_black(parent)
				rb_set_red(gparent)
				node = gparent
				continue
			}
			// Case 2条件：叔叔是黑色，且当前节点是左孩子
			if parent.left == node {
				right_rotate(root, parent)
				var tmp *RBNode = parent
				parent = node
				node = tmp
			}
			// Case 3条件：叔叔是黑色，且当前节点是右孩子。
			rb_set_black(parent)
			rb_set_red(gparent)
			left_rotate(root, gparent)
		}
	}
	// 将根节点设为黑色
	rb_set_black(root.node)
}

func DeleteInRbtree(root *RBRoot, node *RBNode) {
	var child, parent *RBNode
	var color uint8

	// 被删除节点的"左右孩子都不为空"的情况。
	if node.left != nil && node.right != nil {
		// 获取后继节点
		var replace *RBNode = node.right
		for replace.left != nil {
			replace = replace.left
		}

		// "node节点"不是根节点
		if node.parent != nil {
			if node.parent.left == node {
				node.parent.left = replace
			} else {
				node.parent.right = replace
			}
		} else {
			// "node节点"是根节点，更新根节点。
			root.node = replace
		}
		// child是"取代节点"的右孩子，也是需要"调整的节点"。
		// "取代节点"肯定不存在左孩子！因为它是一个后继节点。
		child = replace.right
		parent = replace.parent
		// 保存"取代节点"的颜色(注意这里删掉的是node节点，但实际删掉的颜色是replace的)
		color = replace.color

		// "被删除节点"是"它的后继节点的父节点"
		if parent == node {
			parent = replace
		} else {
			// child不为空
			if child != nil {
				child.parent = parent
			}
			parent.left = child

			replace.right = node.right
			node.right.parent = replace
		}

		replace.parent = node.parent
		replace.color = node.color
		replace.left = node.left
		node.left.parent = replace

		if color == BLACK {
			delete_fixup(root, child, parent)
		}

		return
	}

	if node.left != nil {
		child = node.left
	} else {
		child = node.right
	}

	parent = node.parent
	color = node.color // 保存"取代节点"的颜色

	if child != nil {
		child.parent = parent
	}

	// "node节点"不是根节点
	if parent != nil {
		if parent.left == node {
			parent.left = child
		} else {
			parent.right = child
		}
	} else {
		root.node = child
	}

	if color == BLACK {
		delete_fixup(root, child, parent)
	}

}

func delete_fixup(root *RBRoot, node *RBNode, parent *RBNode) {
	var other *RBNode

	for (node == nil || rb_is_black(node)) && node != root.node {
		// node是父节点的左孩子
		if parent.left == node {
			other = parent.right
			// Case 1: node的兄弟节点是红色的
			if rb_is_red(other) {
				rb_set_black(other)
				rb_set_red(parent)
				left_rotate(root, parent)
				other = parent.right
			}
			// Case 2: node的兄弟w是黑色，且w的俩个孩子也都是黑色的
			if (other.left == nil || rb_is_black(other.left)) && (other.right == nil || rb_is_black(other.right)) {
				rb_set_red(other)
				node = parent
				parent = node.parent
			} else {
				// Case 3: node的兄弟w是黑色的，并且w的左孩子是红色，右孩子为黑色。
				if other.right == nil || rb_is_black(other.right) {
					rb_set_black(other.left)
					rb_set_red(other)
					right_rotate(root, other)
					other = parent.right
				}
				// Case 4: node的兄弟w是黑色的；并且w的右孩子是红色的，左孩子任意颜色。
				other.color = parent.color
				rb_set_black(parent)
				rb_set_black(other.right)
				left_rotate(root, parent)
				node = root.node
				break
			}
		} else {
			other = parent.left
			// Case 1: node的兄弟w是红色的
			if rb_is_red(other) {
				rb_set_black(other)
				rb_set_red(parent)
				right_rotate(root, parent)
				other = parent.left
			}
			// Case 2: node的兄弟w是黑色，且w的俩个孩子也都是黑色的
			if (other.left == nil || rb_is_black(other.left)) && (other.right == nil || rb_is_black(other.right)) {
				rb_set_red(other)
				node = parent
				parent = node.parent
			} else {
				// Case 3: node的兄弟w是黑色的，并且w的左孩子是红色，右孩子为黑色。
				if other.left == nil || rb_is_black(other.left) {
					rb_set_black(other.right)
					rb_set_red(other)
					left_rotate(root, other)
					other = parent.left
				}
				// Case 4: node的兄弟w是黑色的；并且w的右孩子是红色的，左孩子任意颜色。
				other.color = parent.color
				rb_set_black(parent)
				rb_set_black(other.left)
				right_rotate(root, parent)
				node = root.node
				break
			}
		}
	}

	if node != nil {
		rb_set_black(node)
	}
}

func RbtreeTest() {
	var datas = []Type{10, 40, 30, 60, 90, 70, 20, 80, 3}

	var root *RBRoot = new(RBRoot)
	for _, data := range datas {
		var node = &RBNode{key: data}
		AddInRbtree(root, node)
	}

	fmt.Print("前序遍历：")
	PreTraverse(root.node)
	fmt.Print("\n中序遍历")
	InTraverse(root.node)
	fmt.Print("\n后序遍历")
	PostTraverse(root.node)
	fmt.Print("\n\n")

	Print(root.node, root.node.key, 0)
	fmt.Print("\n")

	var delNode = Find(root.node, 30)
	DeleteInRbtree(root, delNode)
	Print(root.node, root.node.key, 0)
	fmt.Print("\n")
	PrintRoute(root.node)

	fmt.Println(FindFirst(root.node))

}
