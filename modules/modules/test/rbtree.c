#include <stdio.h>
#include <stdlib.h>

// 红黑树节点颜色
typedef enum { RED, BLACK } Color;

// 红黑树节点
typedef struct Node {
    int key;
    Color color;
    struct Node *left, *right, *parent;
} Node;

// 创建新节点
Node* createNode(int key) {
    Node* newNode = (Node*)malloc(sizeof(Node));
    newNode->key = key;
    newNode->color = RED;
    newNode->left = newNode->right = newNode->parent = NULL;
    return newNode;
}

// 左旋操作
void leftRotate(Node **root, Node *x) {
    Node *y = x->right;
    x->right = y->left;

    if (x->right != NULL)
        x->right->parent = x;

    y->parent = x->parent;

    if (x->parent == NULL)
        *root = y;
    else if (x == x->parent->left)
        x->parent->left = y;
    else
        x->parent->right = y;

    y->left = x;
    x->parent = y;
}

// 右旋操作
void rightRotate(Node **root, Node *y) {
    Node *x = y->left;
    y->left = x->right;

    if (x->right != NULL)
        x->right->parent = y;

    x->parent = y->parent;

    if (y->parent == NULL)
        *root = x;
    else if (y == y->parent->left)
        y->parent->left = x;
    else
        y->parent->right = x;

    x->right = y;
    y->parent = x;
}

// 插入修正操作
void fixInsert(Node **root, Node *z) {
    while (z != *root && z->parent->color == RED) {
        if (z->parent == z->parent->parent->left) {
            Node *y = z->parent->parent->right;
            if (y != NULL && y->color == RED) {
                z->parent->color = BLACK;
                y->color = BLACK;
                z->parent->parent->color = RED;
                z = z->parent->parent;
            } else {
                if (z == z->parent->right) {
                    z = z->parent;
                    leftRotate(root, z);
                }
                z->parent->color = BLACK;
                z->parent->parent->color = RED;
                rightRotate(root, z->parent->parent);
            }
        } else {
            Node *y = z->parent->parent->left;
            if (y != NULL && y->color == RED) {
                z->parent->color = BLACK;
                y->color = BLACK;
                z->parent->parent->color = RED;
                z = z->parent->parent;
            } else {
                if (z == z->parent->left) {
                    z = z->parent;
                    rightRotate(root, z);
                }
                z->parent->color = BLACK;
                z->parent->parent->color = RED;
                leftRotate(root, z->parent->parent);
            }
        }
    }
    (*root)->color = BLACK;
}

// 插入节点
void insertNode(Node **root, int key) {
    Node *z = createNode(key);
    Node *y = NULL;
    Node *x = *root;

    // 找到插入位置
    while (x != NULL) {
        y = x;
        if (z->key < x->key)
            x = x->left;
        else
            x = x->right;
    }

    // 设置父节点
    z->parent = y;

    if (y == NULL)
        *root = z;
    else if (z->key < y->key)
        y->left = z;
    else
        y->right = z;

    // 修正红黑树性质
    fixInsert(root, z);
}

// 中序遍历红黑树
void inorderTraversal(Node *root) {
    if (root == NULL)
        return;

    inorderTraversal(root->left);
    printf("%d ", root->key);
    inorderTraversal(root->right);
}

// 查找节点
Node* searchNode(Node* root, int key) {
    if (root == NULL || root->key == key)
        return root;

    if (key < root->key)
        return searchNode(root->left, key);
    else
        return searchNode(root->right, key);
}

// 获取树中的最小节点
Node* getMinimumNode(Node* node) {
    while (node->left != NULL)
        node = node->left;
    return node;
}

// 删除修正操作
void fixDelete(Node **root, Node *x) {
    while (x != *root && (x == NULL || x->color == BLACK)) {
        if (x == x->parent->left) {
            Node *w = x->parent->right;
            if (w != NULL && w->color == RED) {
                w->color = BLACK;
                x->parent->color = RED;
                leftRotate(root, x->parent);
                w = x->parent->right;
            }
            if ((w->left == NULL || w->left->color == BLACK) &&
                (w->right == NULL || w->right->color == BLACK)) {
                w->color = RED;
                x = x->parent;
            } else {
                if (w->right == NULL || w->right->color == BLACK) {
                    if (w->left != NULL)
                        w->left->color = BLACK;
                    w->color = RED;
                    rightRotate(root, w);
                    w = x->parent->right;
                }
                w->color = x->parent->color;
                x->parent->color = BLACK;
                if (w->right != NULL)
                    w->right->color = BLACK;
                leftRotate(root, x->parent);
                x = *root;
            }
        } else {
            Node *w = x->parent->left;
            if (w != NULL && w->color == RED) {
                w->color = BLACK;
                x->parent->color = RED;
                rightRotate(root, x->parent);
                w = x->parent->left;
            }
            if ((w->right == NULL || w->right->color == BLACK) &&
                (w->left == NULL || w->left->color == BLACK)) {
                w->color = RED;
                x = x->parent;
            } else {
                if (w->left == NULL || w->left->color == BLACK) {
                    if (w->right != NULL)
                        w->right->color = BLACK;
                    w->color = RED;
                    leftRotate(root, w);
                    w = x->parent->left;
                }
                w->color = x->parent->color;
                x->parent->color = BLACK;
                if (w->left != NULL)
                    w->left->color = BLACK;
                rightRotate(root, x->parent);
                x = *root;
            }
        }
    }
    if (x != NULL)
        x->color = BLACK;
}

// 删除节点
void deleteNode(Node **root, int key) {
    Node *z = searchNode(*root, key);
    if (z == NULL)
        return;

    Node *y = z;
    Node *x;
    Color yOriginalColor = y->color;

    if (z->left == NULL) {
        x = z->right;
        transplant(root, z, z->right);
    } else if (z->right == NULL) {
        x = z->left;
        transplant(root, z, z->left);
    } else {
        y = getMinimumNode(z->right);
        yOriginalColor = y->color;
        x = y->right;
        if (y->parent == z)
            x->parent = y;
        else {
            transplant(root, y, y->right);
            y->right = z->right;
            y->right->parent = y;
        }
        transplant(root, z, y);
        y->left = z->left;
        y->left->parent = y;
        y->color = z->color;
    }

    if (yOriginalColor == BLACK)
        fixDelete(root, x);

    free(z);
}

// 辅助函数：替换节点
void transplant(Node **root, Node *u, Node *v) {
    if (u->parent == NULL)
        *root = v;
    else if (u == u->parent->left)
        u->parent->left = v;
    else
        u->parent->right = v;

    if (v != NULL)
        v->parent = u->parent;
}


// 测试函数
void testRedBlackTree() {
    Node *root = NULL;

    // 插入节点
    insertNode(&root, 7);
    insertNode(&root, 3);
    insertNode(&root, 18);
    insertNode(&root, 10);
    insertNode(&root, 22);
    insertNode(&root, 8);
    insertNode(&root, 11);
    insertNode(&root, 26);

    // 中序遍历红黑树
    printf("inorderTraversal: ");
    inorderTraversal(root);
    printf("\n");

    // 查找节点
    int keyToFind = 10;
    Node *foundNode = searchNode(root, keyToFind);
    if (foundNode != NULL)
        printf("find %d\n", keyToFind);
    else
        printf("not found %d\n", keyToFind);

    // 删除节点
    int keyToDelete = 18;
    deleteNode(&root, keyToDelete);
    printf("delete %d\n", keyToDelete);

    // 中序遍历红黑树
    printf("inorderTraversal: ");
    inorderTraversal(root);
    printf("\n");

    // 再次查找节点
    foundNode = searchNode(root, keyToFind);
    if (foundNode != NULL)
        printf("find %d\n", keyToFind);
    else
        printf("not found %d\n", keyToFind);
}

int main() {
    testRedBlackTree();

    return 0;
}
