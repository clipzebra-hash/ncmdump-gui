import React, { useEffect, useRef, useMemo } from 'react'
import { useState } from 'react'
import {
  Button,
  Checkbox,
  Tab,
  TabList,
  createTableColumn,
  Dialog,
  DialogActions,
  DialogBody,
  DialogContent,
  DialogSurface,
  DialogTitle,
  DialogTrigger,
  Field,
  Input,
  Radio,
  RadioGroup,
  TableColumnDefinition,
  TableColumnSizingOptions,
  useTableColumnSizing_unstable,
  useTableFeatures,
} from '@fluentui/react-components'
import {
  TableBody,
  TableCell,
  TableRow,
  Table,
  TableHeader,
  TableHeaderCell,
  TableCellLayout,
  makeStyles,
  tokens,
} from '@fluentui/react-components'
import {
  ClockRegular,
  CheckmarkRegular,
  DismissRegular,
  DocumentArrowRightRegular,
  DocumentAddRegular,
  DeleteRegular,
  DeleteDismissRegular,
  FolderAddRegular,
  WindowPlayRegular,
} from '@fluentui/react-icons'

import { Status, Item, SaveTo } from './types'
import { SelectFiles, SelectFolder, SelectFilesFromFolder, ProcessFiles, CancelBatch, GetProgress } from '../wailsjs/go/main/App'
import { Load, Save } from '../wailsjs/go/utils/ConfigManager'
import { main } from '../wailsjs/go/models'
import { EventsOn, OnFileDrop, OnFileDropOff } from '../wailsjs/runtime/runtime'

const columnsDef: TableColumnDefinition<Item>[] = [
  createTableColumn<Item>({
    columnId: 'status',
    renderHeaderCell: () => <>状态</>,
  }),
  createTableColumn<Item>({
    columnId: 'file',
    renderHeaderCell: () => <>文件</>,
  }),
  createTableColumn<Item>({
    columnId: 'operation',
    renderHeaderCell: () => <>操作</>,
  }),
]

const useStyles = makeStyles({
  iconGreen: {
    color: tokens.colorPaletteGreenForeground1,
  },
  iconRed: {
    color: tokens.colorPaletteRedForeground1,
  },
  iconYellow: {
    color: tokens.colorPaletteYellowForeground1,
  },
})

let loaded = false

export const App = () => {
  const styles = useStyles()

  const [items, setItems] = useState<Item[]>([])
  const [isProcessing, setProcessing] = useState(false)
  const busy = useRef(false)
  const accepting = useRef(false)
  const generation = useRef(0)
  const [progress, setProgress] = useState<Omit<main.BatchProgress, "convertValues">>({Deleted:0,Errors:[],Total:0,Completed:0,Success:0,Failed:0,Current:'',Elapsed:0,Running:false,Cancelled:false,Warning:'',Error:''})
  const [cancelling, setCancelling] = useState(false)
  const errorsByIndex = useMemo(()=>new Map((progress.Errors || []).map(e=>[e.Index,e.Message])),[progress.Errors])
  const [elapsed, setElapsed] = useState(0)
  const started = useRef(0)
  const addFiles = (files: string[]) => {
    if (busy.current) return
    setItems(prev => {
      const seen = new Set(prev.map(i=>i.file))
      return [...prev,...files.filter(f=>/\.ncm$/i.test(f) && !seen.has(f) && !!seen.add(f)).map(file=>({file,status:'pending' as Status}))]
    })
  }

  const [page, setPage] = useState<"convert" | "options">("convert")
  const [deleteSource, setDeleteSource] = useState(false)
  const [saveTo, setSaveTo] = useState<SaveTo>('original')
  const [savePath, setSavePath] = useState('')

  const [message, setMessage] = useState('')
  const [open, setOpen] = useState(false)

  const [columns] = React.useState<TableColumnDefinition<Item>[]>(columnsDef)
  const [columnSizingOptions] = React.useState<TableColumnSizingOptions>({
    status: {
      idealWidth: 100,
      minWidth: 100,
    },
    file: {
      idealWidth: 1000,
      minWidth: 150,
    },
    operation: {
      idealWidth: 80,
      minWidth: 80,
    },
  })

  // eslint-disable-next-line @typescript-eslint/naming-convention
  const { getRows, columnSizing_unstable, tableRef } = useTableFeatures(
    {
      columns,
      items,
    },
    [useTableColumnSizing_unstable({ columnSizingOptions })]
  )

  // return the icon based on the status
  const statusMapIcon = (status: Status) => {
    switch (status) {
      case 'pending':
        return <ClockRegular />
      case 'processing':
        return <DocumentArrowRightRegular className={styles.iconYellow} />
      case 'done':
        return <CheckmarkRegular className={styles.iconGreen} />
      case 'error':
        return <DismissRegular className={styles.iconRed} />
      default:
    }
  }

  // return the text based on the status
  const statusMapText = (status: Status) => {
    switch (status) {
      case 'pending':
        return '等待'
      case 'processing':
        return '处理'
      case 'done':
        return '完成'
      case 'error':
        return '错误'
    }
  }

  const selectFiles = () => {
    if (busy.current) {
      return
    }
    SelectFiles().then(files => {
      addFiles(files)
    })
  }

  const selectFilesFromFolder = () => {
    if (busy.current) {
      return
    }
    SelectFilesFromFolder('ncm').then(files => {
      addFiles(files)
    })
  }

  const showDialog = (message: string) => {
    setMessage(message)
    setOpen(true)
  }

  const startProcess = async () => {
    if (busy.current) return
    // 未添加文件时
    if (items.length === 0) {
      showDialog('当前文件列表为空，请先添加文件。')
      return
    }
    // 检查所有文件是否都已处理完毕
    let isAllFinished = true
    for (const item of items) {
      if (item.status === 'pending') {
        isAllFinished = false
        break
      }
    }
    if (isAllFinished) {
      showDialog('当前文件列表已全部处理完毕，请重新添加新的文件。')
      return
    }
    if (saveTo === 'custom' && savePath === '') {
      showDialog('保存路径为空，请先设置保存路径。')
      return
    }
    const ncmFiles: main.NcmFile[] = []
    items.forEach(item => {
      ncmFiles.push({
        Name: item.file,
        Status: item.status,
      })
    })
    busy.current = true
    accepting.current = true
    generation.current++
    setProcessing(true)
    setCancelling(false)
    started.current = Date.now()
    setElapsed(0)
    setProgress({Deleted:0,Errors:[],Total:items.filter(i=>i.status==='pending').length,Completed:0,Success:0,Failed:0,Current:'',Elapsed:0,Running:true,Cancelled:false,Warning:'',Error:''})
    try { await ProcessFiles(ncmFiles, saveTo === 'original' ? '' : savePath, deleteSource) }
    catch (e) { busy.current=false;setProcessing(false);showDialog(String(e)) }
    finally { accepting.current = false }
  }

  useEffect(() => {
    const off = EventsOn('file-status-changed', (index: number, status: Status) => {
      setItems(prev => {
        const newItems = [...prev]
        if (!newItems[index]) return prev
        newItems[index] = {...newItems[index],status}
        return newItems
      })
    })
    const poll = setInterval(() => {
      const requestedGeneration = generation.current
      GetProgress().then(p => {
        if (accepting.current || requestedGeneration !== generation.current) return
        setProgress(p)
        // An active backend snapshot also restores state after a view refresh.
        if (p.Running) { busy.current=true;setProcessing(true);setElapsed(p.Elapsed || (Date.now()-started.current)/1000) }
        else if (busy.current) {busy.current=false;setProcessing(false);setElapsed(p.Elapsed)}
      }).catch(()=>{})
    }, 250)
    Load().then(res => {
      if (!res) return
      setDeleteSource(res.delete_source ?? false)
      setSaveTo(res.save_to as SaveTo)
      setSavePath(res.path)
      loaded = true
    })
    return () => {off();clearInterval(poll)}
  }, [])

  useEffect(() => {
    if (loaded) {
      Save({
        delete_source: deleteSource,
        save_to: saveTo,
        path: savePath,
      }).then(_ => {})
    }
  }, [saveTo, savePath, deleteSource])

  useEffect(() => {
    OnFileDrop((_x, _y, paths) => addFiles(paths), false)
    return () => OnFileDropOff()
  }, [])

  return (
    <div className="p-3">
      <Dialog
        // this controls the dialog open state
        open={open}
        onOpenChange={(event, data) => {
          // it is the users responsibility to react accordingly to the open state change
          setOpen(data.open)
        }}
      >
        <DialogSurface style={{ width: '400px' }}>
          <DialogBody>
            <DialogTitle>警告</DialogTitle>
            <DialogContent>{message}</DialogContent>
            <DialogActions>
              <DialogTrigger disableButtonEnhancement>
                <Button appearance="primary">关闭</Button>
              </DialogTrigger>
            </DialogActions>
          </DialogBody>
        </DialogSurface>
      </Dialog>
      <TabList selectedValue={page} onTabSelect={(_,data)=>setPage(data.value as "convert" | "options")}>
        <Tab value="convert">转换</Tab>
        <Tab value="options">选项</Tab>
      </TabList>
      <section hidden={page !== 'options'} aria-label="选项">
      <div className="mt-3">
        <Field label="保存转换后的文件到">
          <RadioGroup
            disabled={isProcessing}
            layout="horizontal"
            value={saveTo}
            onChange={(_, data) => {
              setSaveTo(data.value as SaveTo)
              if (data.value === 'original') {
                setSavePath('')
              }
            }}
          >
            <Radio value="original" label="源文件所在目录" />
            <Radio value="custom" label="自定义保存目录" />
            {saveTo === 'custom' && (
              <Input
                placeholder="点击来选择保存目录"
                value={savePath}
                readOnly
                style={{ flexGrow: 1 }}
                onClick={() => {
                  if (busy.current) return
                  SelectFolder().then(path => {
                    if (path) {
                      setSavePath(path)
                    }
                  })
                }}
              />
            )}
          </RadioGroup>
        </Field>
      </div>
        <div className="mt-3">
          <Checkbox disabled={isProcessing} checked={deleteSource} onChange={(_,data)=>setDeleteSource(data.checked === true)} label="转换成功后删除源 NCM（移至废纸篓）" />
          <p>默认关闭。开启后，成品保存并通过读回校验才移走源文件；失败、取消或已有同名输出时保留源文件。可从废纸篓恢复。</p>
        </div>
      </section>
      <section hidden={page !== 'convert'} aria-label="转换">
      <div className="mt-3" style={{overflowWrap:'anywhere'}}>保存到：{saveTo === 'original' ? '源文件所在目录' : savePath || '未设置'} · {deleteSource ? '成功后源 NCM 移至废纸篓' : '保留源 NCM'}</div>
      <div className="flex space-between gap-3">
        <Button onClick={selectFiles} icon={<DocumentAddRegular />}>
          添加文件
        </Button>
        <Button onClick={selectFilesFromFolder} icon={<FolderAddRegular />}>添加目录</Button>
        <Button
          onClick={() => {
            if (!busy.current) {
              setItems([])
            }
          }}
          icon={<DeleteDismissRegular />}
        >
          清除列表
        </Button>
        <Button
          appearance="primary"
          icon={<WindowPlayRegular />}
          onClick={() => {
            startProcess()
          }}
          disabled={isProcessing}
        >
          {isProcessing ? '处理中...' : '开始处理'}
        </Button>
      </div>
      {progress.Total > 0 && <div className="mt-3" aria-live="polite">
        <progress style={{width:'100%'}} max={progress.Total} value={progress.Completed} aria-label="总体转换进度" />
        <div>{progress.Completed}/{progress.Total} · 成功 {progress.Success} · 失败 {progress.Failed} · 已移至废纸篓 {progress.Deleted} · 耗时 {elapsed.toFixed(1)} 秒 {progress.Cancelled ? '· 已取消，未完成文件可继续' : ''}</div>
        <div style={{overflowWrap:'anywhere'}}>当前文件：{progress.Current || '—'}</div>
        <div>仅使用文件内嵌封面，不联网下载。</div>
        {progress.Error && <div role="alert">{progress.Error}</div>}
        <Button disabled={!isProcessing || cancelling} onClick={()=>{setCancelling(true);CancelBatch().catch(e=>{setCancelling(false);showDialog(String(e))})}}>{cancelling && isProcessing ? "正在取消…" : "取消（保留已完成文件）"}</Button>
      </div>}

      <Table
        ref={tableRef}
        arial-label="Default table"
        style={{ minWidth: '510px' }}
        size="small"
        className="mt-3"
      >
        <TableHeader>
          <TableRow>
            {columns.map(column => (
              <TableHeaderCell
                key={column.columnId}
                {...columnSizing_unstable.getTableHeaderCellProps(column.columnId)}
              >
                {column.renderHeaderCell()}
              </TableHeaderCell>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((file, index) => (
            <TableRow key={index}>
              <TableCell>
                <TableCellLayout media={statusMapIcon(file.status)}>
                  {statusMapText(file.status)}
                </TableCellLayout>
              </TableCell>
              <TableCell>{file.file}{errorsByIndex.has(index) && <div role="alert" style={{color:tokens.colorPaletteRedForeground1,overflowWrap:"anywhere"}}>{errorsByIndex.get(index)}</div>}</TableCell>
              <TableCell>
                <Button
                  disabled={isProcessing}
                  size="small"
                  icon={<DeleteRegular />}
                  appearance="transparent"
                  onClick={() => {
                    if (busy.current) return
                    setItems(prev => prev.filter((_, i) => i !== index))
                  }}
                >
                  移除
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      </section>
    </div>
  )
}

export default App
