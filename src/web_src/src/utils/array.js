export function arrayToTree(arr) {
  // init resulting array
  const res = []
  // use Map save array elements，key: id，value: itself
  const map = new Map()

  // iterate array, save each element to the map
  arr.forEach((item) => {
    map.set(item.id, item)
  })

  // 再次遍历数组，根据pid将元素组织成树形结构
  arr.forEach((item) => {
    // 获取当前元素的父级元素
    const parent = item.pid && map.get(item.pid)
    // 如果有父级元素
    if (parent) {
      // 如果父级元素已有子元素，则将当前元素追加到子元素数组中
      if (parent?.children)
        parent.children.push(item)
      // 如果父级元素没有子元素，则创建子元素数组，并将当前元素作为第一个元素
      else
        parent.children = [item]
    }
    // 如果没有父级元素，则将当前元素直接添加到结果数组中
    else {
      res.push(item)
    }
  })
  // 返回组织好的树形结构数组
  return res
}
