import { Routes, Route, NavLink } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import TableView from './pages/TableView'
import CreateTable from './pages/CreateTable'
import ImportWizard from './pages/ImportWizard'
import QueryEditor from './pages/QueryEditor'
import SchemaList from './pages/SchemaList'

export default function App() {
  return (
    <div className="app">
      <aside className="sidebar">
        <div className="sidebar-header">
          <h1>ISAM Admin</h1>
          <p>Micro Focus ISAM Manager</p>
        </div>
        <nav>
          <NavLink to="/" end>
            <span>Dashboard</span>
          </NavLink>
          <NavLink to="/import">
            <span>Import Wizard</span>
          </NavLink>
          <NavLink to="/create">
            <span>Create Table</span>
          </NavLink>
          <NavLink to="/schemas">
            <span>Schemas</span>
          </NavLink>
          <NavLink to="/query">
            <span>Query</span>
          </NavLink>
        </nav>
      </aside>
      <main className="main-content">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/table/:name" element={<TableView />} />
          <Route path="/import" element={<ImportWizard />} />
          <Route path="/create" element={<CreateTable />} />
          <Route path="/schemas" element={<SchemaList />} />
          <Route path="/query" element={<QueryEditor />} />
        </Routes>
      </main>
    </div>
  )
}
